package models

import (
	"net/http"
	"net/url"
	pb "ucode/ucode_go_client_api_gateway/genproto/company_service"
	"ucode/ucode_go_client_api_gateway/genproto/new_function_service"
)

type Function struct {
	ID               string `json:"id"`
	Path             string `json:"path"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	FuncitonFolderId string `json:"function_folder_id"`
}

type CreateFunctionRequest struct {
	Path             string `json:"path"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	CommitId         int64  `json:"-"`
	CommitGuid       string `json:"-"`
	VersionId        string `json:"-"`
	FunctionFolderId string `json:"function_folder_id"`
	FrameworkType    string `json:"framework_type"`
}

type InvokeFunctionRequest struct {
	FunctionID string   `json:"function_id"`
	ObjectIDs  []string `json:"object_ids"`
	Attributes map[string]any
}

type InvokeFunctionResponse struct {
	Status      string                 `json:"status"`
	Data        map[string]any `json:"data"`
	Attributes  map[string]any `json:"attributes"`
	ServerError string                 `json:"server_error"`
}

type GetListClientApiResp struct {
	Response       []map[string]any `json:"response"`
	Fields         []map[string]any `json:"fields"`
	Views          []map[string]any `json:"views"`
	RelationFields []map[string]any `json:"relation_fields"`
}

type InvokeFunctionResponse2 struct {
	Status string               `json:"status"`
	Data   GetListClientApiResp `json:"data"`
}

type NewInvokeFunctionRequest struct {
	Data map[string]any `json:"data"`
}

type HttpRequest struct {
	Method  string      `json:"method"`
	Path    string      `json:"path"`
	Headers http.Header `json:"headers"`
	Params  url.Values  `json:"params"`
	Body    []byte      `json:"body"`
}

type AuthData struct {
	Type string                 `json:"type"`
	Data map[string]any `json:"data"`
}

type FunctionRunV2 struct {
	RequestData HttpRequest            `json:"request_data"`
	Auth        AuthData               `json:"auth"`
	Data        map[string]any `json:"data"`
}

type InvokeFunctionRequestWithAppId struct {
	ObjectIDs []string `json:"object_ids"`
	AppID     string   `json:"app_id"`
}

type GetByIdFunctionResponse struct {
	Password         string `json:"password"`
	URL              string `json:"url"`
	ID               string `json:"id"`
	Path             string `json:"path"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	FuncitonFolderId string `json:"function_folder_id"`
}

type MicrofrontForLoginPage struct {
	Function      *new_function_service.Function `json:"function"`
	Id            string                         `json:"id"`
	ProjectId     string                         `json:"project_id"`
	EnvironmentId string                         `json:"environment_id"`
	MicrofrontId  string                         `json:"microfront_id"`
	Subdomain     string                         `json:"subdomain"`
}

type GetListCustomEventsStruct struct {
	TableSlug string
	RoleId    string
	Method    string
	Resource  *pb.ServiceResourceModel
}
