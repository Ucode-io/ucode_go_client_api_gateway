package v2

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"ucode/ucode_go_client_api_gateway/api/models"
	"ucode/ucode_go_client_api_gateway/api/status_http"
	"ucode/ucode_go_client_api_gateway/config"
	"ucode/ucode_go_client_api_gateway/pkg/util"

	"github.com/gin-gonic/gin"
)

const (
	hasuraMetadataURL = "http://localhost:8080/v1/metadata"
	hasuraGraphQLURL  = "http://localhost:8080/v1/graphql"
	adminSecret       = "snZV9XNmvf" // Replace with your actual admin secret
)

func (h *HandlerV2) Graphql(c *gin.Context) {
	var (
		respBody []byte
	)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/v1/graphql", h.baseConf.HasuraBaseURL), bytes.NewBuffer(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create request to Hasura"})
		return
	}

	for name, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	req.Header.Set("x-hasura-admin-secret", h.baseConf.HasuraAdminSecret)
	req.Header.Set("Content-Type", "application/json")

	// Make the request to Hasura
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to connect to Hasura"})
		return
	}
	defer resp.Body.Close()

	respHeader := resp.Header.Get("Content-Encoding")
	switch respHeader {
	case "gzip":
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decompress GZIP"})
			return
		}
		defer reader.Close()

		respBody, err = io.ReadAll(reader)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read Hasura response"})
			return
		}
	default:
		respBody, err = io.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read Hasura response"})
			return
		}

	}

	c.Data(resp.StatusCode, "application/json", respBody)
}

func (h *HandlerV2) HasuraEvent(c *gin.Context) {
	var (
		event map[string]any
		path  string = c.Param("path")
	)
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	resp, err := h.ExecKnative(path, models.NewInvokeFunctionRequest{Data: event})
	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	} else if resp.Status == "error" {
		var errStr = resp.Status
		if resp.Data != nil && resp.Data["message"] != nil {
			errStr = resp.Data["message"].(string)
		}
		h.handleResponse(c, status_http.InvalidArgument, errStr)
		return
	}
}

func (h *HandlerV2) ExecKnative(path string, req models.NewInvokeFunctionRequest) (models.InvokeFunctionResponse, error) {
	url := fmt.Sprintf("http://%s.%s", path, config.KnativeBaseUrl)
	resp, err := util.DoRequest(url, http.MethodPost, req)
	if err != nil {
		return models.InvokeFunctionResponse{}, err
	}

	return resp, nil
}

func GenerateREST() {
	tables, err := getTrackedTables()
	if err != nil {
		log.Fatalf("Failed to get tracked tables: %v", err)
	}

	for _, table := range tables {
		if table == "jupiter" {
			fmt.Printf("Processing table: %s\n", table)
			fields, pkFields, err := getTableSchema(table)
			if err != nil {
				log.Printf("Error getting schema for %s: %v", table, err)
				continue
			}

			operations := generateQueriesMutations(table, fields, pkFields)
			payload := generateBulkPayload(table, operations)

			err = applyBulkOperation(payload)
			if err != nil {
				log.Printf("Failed to create REST endpoints for %s: %v", table, err)
			} else {
				log.Printf("Successfully created REST endpoints for %s", table)
			}
		}
	}

	log.Println("REST endpoint creation completed!")
}

func getTrackedTables() ([]string, error) {
	payload := map[string]any{
		"type":    "export_metadata",
		"version": 2,
		"args":    map[string]any{},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", hasuraMetadataURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hasura-Admin-Secret", adminSecret)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch metadata: %s", string(body))
	}

	var metadata models.MetadataResponse
	err = json.NewDecoder(resp.Body).Decode(&metadata)
	if err != nil {
		return nil, err
	}

	var tables []string
	for _, table := range metadata.Metadata.Sources[0].Tables {
		tables = append(tables, table.Table.Name)
	}

	return tables, nil
}

func getTableSchema(tableName string) ([]string, []string, error) {
	introspectionQuery := `
		query IntrospectionQuery($tableName: String!, $pkTypeName: String!) {
			tableType: __type(name: $tableName) {
				name
				fields {
					name
					type {
						name
						kind
					}
				}
			}
			pkType: __type(name: $pkTypeName) {
				name
				fields {
					name
					type {
						name
						kind
					}
				}
			}
		}`

	variables := map[string]any{
		"tableName":  tableName,
		"pkTypeName": fmt.Sprintf("%s_pk_columns_input", tableName),
	}

	requestBody := map[string]any{
		"query":     introspectionQuery,
		"variables": variables,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("POST", hasuraGraphQLURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hasura-Admin-Secret", adminSecret)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("failed to fetch schema for %s: %s", tableName, string(body))
	}

	var result models.GraphQLResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, nil, err
	}

	if len(result.Errors) > 0 {
		return nil, nil, fmt.Errorf("GraphQL error for %s: %v", tableName, result.Errors)
	}

	var fields []string
	for _, field := range result.Data.TableType.Fields {
		currentType := field.Type

		if currentType.Kind == "OBJECT" {
			fmt.Printf("Skipping object relationship field %s in %s (related to %s)\n",
				field.Name, tableName, currentType.Name)
			continue
		}

		fields = append(fields, field.Name)
	}

	var pkFields []string
	for _, field := range result.Data.PKType.Fields {
		pkFields = append(pkFields, field.Name)
	}

	return fields, pkFields, nil
}

func generateQueriesMutations(tableName string, fields, pkFields []string) map[string]struct {
	Name  string
	Query string
} {
	pkField := "guid"
	pkType := "uuid"

	fieldsStr := ""
	for _, field := range fields {
		fieldsStr += "    " + field + "\n"
	}

	operations := map[string]struct {
		Name  string
		Query string
	}{
		"read": {
			Name: fmt.Sprintf("%s_by_pk", tableName),
			Query: fmt.Sprintf(`
query %s_by_pk($%s: %s!) {
  %s_by_pk(%s: $%s) {
    %s
  }
}`, tableName, pkField, pkType, tableName, pkField, pkField, fieldsStr),
		},
		"read_all": {
			Name: tableName,
			Query: fmt.Sprintf(`
query %s {
  %s {
    %s
  }
}`, tableName, tableName, fieldsStr),
		},
		"create": {
			Name: fmt.Sprintf("insert_%s_one", tableName),
			Query: fmt.Sprintf(`
mutation insert_%s_one($object: %s_insert_input!) {
  insert_%s_one(object: $object) {
    %s
  }
}`, tableName, tableName, tableName, fieldsStr),
		},
		"update": {
			Name: fmt.Sprintf("update_%s_by_pk", tableName),
			Query: fmt.Sprintf(`
mutation update_%s_by_pk($%s: %s!, $object: %s_set_input!) {
  update_%s_by_pk(pk_columns: {%s: $%s}, _set: $object) {
    %s
  }
}`, tableName, pkField, pkType, tableName, tableName, pkField, pkField, fieldsStr),
		},
		"delete": {
			Name: fmt.Sprintf("delete_%s_by_pk", tableName),
			Query: fmt.Sprintf(`
mutation delete_%s_by_pk($%s: %s!) {
  delete_%s_by_pk(%s: $%s) {
    %s
  }
}`, tableName, pkField, pkType, tableName, pkField, pkField, fieldsStr),
		},
	}

	return operations
}

func generateBulkPayload(tableName string, operations map[string]struct {
	Name  string
	Query string
}) models.BulkOperation {
	var bulkArgs []any

	for _, op := range operations {
		bulkArgs = append(bulkArgs, map[string]any{
			"type": "add_query_to_collection",
			"args": map[string]any{
				"collection_name": "allowed-queries",
				"query_name":      op.Name,
				"query":           op.Query,
			},
		})
	}

	for opName, op := range operations {
		endpointArgs := map[string]any{
			"name": op.Name,
			"definition": map[string]any{
				"query": map[string]string{
					"query_name":      op.Name,
					"collection_name": "allowed-queries",
				},
			},
			"comment": "",
		}

		switch opName {
		case "read":
			endpointArgs["url"] = fmt.Sprintf("%s/:guid", tableName)
			endpointArgs["methods"] = []string{http.MethodGet}
		case "read_all":
			endpointArgs["url"] = tableName
			endpointArgs["methods"] = []string{http.MethodGet}
		case "create":
			endpointArgs["url"] = tableName
			endpointArgs["methods"] = []string{http.MethodPost}
		case "update":
			endpointArgs["url"] = fmt.Sprintf("%s/:guid", tableName)
			endpointArgs["methods"] = []string{http.MethodPut}
		case "delete":
			endpointArgs["url"] = fmt.Sprintf("%s/:guid", tableName)
			endpointArgs["methods"] = []string{http.MethodDelete}
		}

		bulkArgs = append(bulkArgs, map[string]any{
			"type": "create_rest_endpoint",
			"args": endpointArgs,
		})
	}

	return models.BulkOperation{
		Type: "bulk",
		Args: bulkArgs,
	}
}

func applyBulkOperation(payload models.BulkOperation) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, hasuraMetadataURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hasura-Admin-Secret", adminSecret)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to apply bulk operation: %s", string(body))
	}

	return nil
}
