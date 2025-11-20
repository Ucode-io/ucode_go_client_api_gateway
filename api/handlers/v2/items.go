package v2

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ucode/ucode_go_client_api_gateway/api/models"
	"ucode/ucode_go_client_api_gateway/api/status_http"
	"ucode/ucode_go_client_api_gateway/config"
	pb "ucode/ucode_go_client_api_gateway/genproto/company_service"
	nb "ucode/ucode_go_client_api_gateway/genproto/new_object_builder_service"
	obs "ucode/ucode_go_client_api_gateway/genproto/object_builder_service"
	"ucode/ucode_go_client_api_gateway/pkg/helper"
	"ucode/ucode_go_client_api_gateway/pkg/logger"
	"ucode/ucode_go_client_api_gateway/pkg/security"
	"ucode/ucode_go_client_api_gateway/pkg/util"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spf13/cast"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// GetAllItems godoc
// @Security ApiKeyAuth
// @ID get_list_items
// @Router /v2/items/{collection} [GET]
// @Summary Get all items
// @Description Get all items
// @Tags Items
// @Accept json
// @Produce json
// @Param collection path string true "collection"
// @Param language_setting query string false "language_setting"
// @Param data query string false "data"
// @Success 200 {object} status_http.Response{data=models.CommonMessage} "ObjectBody"
// @Response 400 {object} status_http.Response{data=string} "Invalid Argument"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) GetAllItems(c *gin.Context) {
	var (
		objectRequest models.CommonMessage
		queryData     string
		statusHttp    = status_http.GrpcStatusToHTTP["Ok"]
		hashed        bool
	)

	queryParams := c.Request.URL.Query()
	if ok := queryParams.Has("data"); ok {
		queryData = queryParams.Get("data")
	}

	if ok := queryParams.Has("data"); ok {
		hashData, err := security.Decrypt(queryParams.Get("data"), h.baseConf.SecretKey)
		if err == nil {
			queryData = strings.TrimSpace(hashData)
			hashed = true
		}
	}

	queryMap := make(map[string]interface{})
	if err := json.Unmarshal([]byte(queryData), &queryMap); err != nil {
		h.handleResponse(c, status_http.BadRequest, err.Error())
		return
	}

	offset, err := h.getOffsetParam(c)
	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}

	limit, err := h.getLimitParam(c)
	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}

	projectId, ok := c.Get("project_id")
	if !ok || !util.IsValidUUID(projectId.(string)) {
		h.handleResponse(c, status_http.InvalidArgument, "project id is an invalid uuid")
		return
	}

	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		h.handleResponse(c, status_http.BadRequest, "error getting environment id | not valid")
		return
	}

	queryMap["limit"] = limit
	queryMap["offset"] = offset

	objectRequest.Data = queryMap
	tokenInfo, err := h.GetAuthInfo(c)
	if err != nil {
		h.handleResponse(c, status_http.Forbidden, err.Error())
		return
	}

	objectRequest.Data["tables"] = tokenInfo.GetTables()
	objectRequest.Data["user_id_from_token"] = tokenInfo.GetUserId()
	objectRequest.Data["role_id_from_token"] = tokenInfo.GetRoleId()
	objectRequest.Data["client_type_id_from_token"] = tokenInfo.GetClientTypeId()
	structData, err := helper.ConvertMapToStruct(objectRequest.Data)
	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}

	userId, _ := c.Get("user_id")
	apiKey := c.GetHeader("X-API-KEY")

	var resource *pb.ServiceResourceModel
	resourceBody, ok := c.Get("resource")
	if resourceBody != "" && ok {
		var resourceList *pb.GetResourceByEnvIDResponse
		err = json.Unmarshal([]byte(resourceBody.(string)), &resourceList)
		if err != nil {
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}

		for _, resourceObject := range resourceList.ServiceResources {
			if resourceObject.Title == pb.ServiceType_name[1] {
				resource = &pb.ServiceResourceModel{
					Id:                    resourceObject.Id,
					ServiceType:           resourceObject.ServiceType,
					ProjectId:             resourceObject.ProjectId,
					Title:                 resourceObject.Title,
					ResourceId:            resourceObject.ResourceId,
					ResourceEnvironmentId: resourceObject.ResourceEnvironmentId,
					EnvironmentId:         resourceObject.EnvironmentId,
					ResourceType:          resourceObject.ResourceType,
					NodeType:              resourceObject.NodeType,
				}
				break
			}
		}
	} else {
		resource, err = h.companyServices.ServiceResource().GetSingle(
			c.Request.Context(), &pb.GetSingleServiceResourceReq{
				ProjectId:     projectId.(string),
				EnvironmentId: environmentId.(string),
				ServiceType:   pb.ServiceType_BUILDER_SERVICE,
			},
		)
		if err != nil {
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}
	}

	services, err := h.GetProjectSrvc(c.Request.Context(), projectId.(string), resource.NodeType)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	var (
		logReq = &models.CreateVersionHistoryRequest{
			Services:     services,
			NodeType:     resource.NodeType,
			ProjectId:    resource.ResourceEnvironmentId,
			ActionSource: c.Request.URL.String(),
			ActionType:   http.MethodGet,
			UserInfo:     cast.ToString(userId),
			Request:      &structData,
			ApiKey:       apiKey,
			Type:         "API_KEY",
			TableSlug:    c.Param("collection"),
		}
	)

	switch resource.ResourceType {
	case pb.ResourceType_MONGODB:
		service := services.GetBuilderServiceByType(resource.NodeType)

		accessType, ok := c.Get("access")
		if ok && cast.ToString(accessType) == config.PublicStatus {
			permission, err := service.Permission().GetTablePermission(
				c.Request.Context(),
				&obs.GetTablePermissionRequest{
					TableSlug:             c.Param("collection"),
					ResourceEnvironmentId: resource.ResourceEnvironmentId,
					Method:                "read",
				},
			)
			if err != nil {
				h.handleResponse(c, status_http.GRPCError, err.Error())
				return
			}

			if !permission.IsHavePermission {
				h.handleResponse(c, status_http.Forbidden, "table is not public")
				return
			}
		}

		var slimKey = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("slim-%s-%s-%s", c.Param("collection"), structData.String(), resource.ResourceEnvironmentId)))
		if !cast.ToBool(c.Query("block_cached")) {
			if cast.ToBool(c.Query("is_wait_cached")) {
				var slimWaitKey = config.CACHE_WAIT + "-slim"
				_, slimOK := h.cache.Get(slimWaitKey)
				if !slimOK {
					h.cache.Add(slimWaitKey, []byte(slimWaitKey), 15*time.Second)
				}

				if slimOK {
					ctx, cancel := context.WithTimeout(c.Request.Context(), config.REDIS_WAIT_TIMEOUT)
					defer cancel()

					for {
						slimBody, ok := h.cache.Get(slimKey)
						if ok {
							m := make(map[string]interface{})
							err = json.Unmarshal(slimBody, &m)
							if err != nil {
								h.handleResponse(c, status_http.GRPCError, err.Error())
								return
							}

							h.handleResponse(c, status_http.OK, map[string]interface{}{"data": m})
							return
						}

						if ctx.Err() == context.DeadlineExceeded {
							break
						}

						time.Sleep(config.REDIS_SLEEP)
					}
				}
			} else {
				redisResp, err := h.redis.Get(c.Request.Context(), slimKey, projectId.(string), resource.NodeType)
				if err == nil {
					resp := make(map[string]interface{})
					m := make(map[string]interface{})
					err = json.Unmarshal([]byte(redisResp), &m)
					if err != nil {
						h.log.Error("Error while unmarshal redis", logger.Error(err))
					} else {
						resp["data"] = m
						h.handleResponse(c, status_http.OK, resp)
						logReq.Response = m
						go h.versionHistory(logReq)
						return
					}
				} else {
					h.log.Error("Error while getting redis while get list ", logger.Error(err))
				}
			}
		}

		resp, err := service.ObjectBuilder().GetListSlimV2(
			c.Request.Context(),
			&obs.CommonMessage{
				TableSlug: c.Param("collection"),
				Data:      structData,
				ProjectId: resource.ResourceEnvironmentId,
			},
		)
		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			logReq.Response = err.Error()
			go h.versionHistory(logReq)
			h.handleResponse(c, statusHttp, err.Error())
			return
		}

		logReq.Response = resp
		go h.versionHistory(logReq)

		if !cast.ToBool(c.Query("block_cached")) {
			jsonData, _ := resp.GetData().MarshalJSON()
			if cast.ToBool(c.Query("is_wait_cached")) {
				h.cache.Add(slimKey, jsonData, 15*time.Second)
			} else if resp.IsCached {
				err = h.redis.SetX(context.Background(), slimKey, string(jsonData), 15*time.Second, projectId.(string), resource.NodeType)
				if err != nil {
					h.log.Error("Error while setting redis", logger.Error(err))
				}
			}
		}

		statusHttp.CustomMessage = resp.GetCustomMessage()

		if hashed {
			hash, err := security.Encrypt(resp, h.baseConf.SecretKey)
			if err != nil {
				h.handleResponse(c, status_http.InternalServerError, err.Error())
				return
			}

			h.handleResponse(c, statusHttp, hash)
			return
		}
		h.handleResponse(c, statusHttp, resp)
	case pb.ResourceType_POSTGRESQL:
		resp, err := services.GoObjectBuilderService().ObjectBuilder().GetListSlim(
			c.Request.Context(), &nb.CommonMessage{
				TableSlug: c.Param("collection"),
				Data:      structData,
				ProjectId: resource.ResourceEnvironmentId,
			},
		)

		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			logReq.Response = err.Error()
			go h.versionHistoryGo(c, logReq)
			h.handleResponse(c, statusHttp, err.Error())
			return
		}

		logReq.Response = resp
		go h.versionHistoryGo(c, logReq)

		statusHttp.CustomMessage = resp.GetCustomMessage()
		h.handleResponse(c, statusHttp, resp)
	}

}

// GetSingleItem godoc
// @Security ApiKeyAuth
// @ID get_item_by_id
// @Router /v2/items/{collection}/{id} [GET]
// @Summary Get item by id
// @Description Get item by id
// @Tags Items
// @Accept json
// @Produce json
// @Param collection path string true "collection"
// @Param id path string true "id"
// @Success 200 {object} status_http.Response{data=models.CommonMessage} "ObjectBody"
// @Response 400 {object} status_http.Response{data=string} "Invalid Argument"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) GetSingleItem(c *gin.Context) {
	var (
		object     models.CommonMessage
		statusHttp = status_http.GrpcStatusToHTTP["Ok"]
		hashed     bool
	)

	object.Data = make(map[string]interface{})

	objectID := c.Param("id")
	if !util.IsValidUUID(objectID) {
		hashData, err := security.Decrypt(c.Param("id"), h.baseConf.SecretKey)
		if err == nil {
			objectID = hashData
			hashed = true
		} else {
			h.handleResponse(c, status_http.InvalidArgument, "object_id is an invalid uuid")
			return
		}
	}

	object.Data["id"] = objectID
	object.Data["with_relations"] = c.DefaultQuery("with_relations", "false")

	structData, err := helper.ConvertMapToStruct(object.Data)
	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}

	projectId, ok := c.Get("project_id")
	if !ok || !util.IsValidUUID(projectId.(string)) {
		h.handleResponse(c, status_http.InvalidArgument, "project id is an invalid uuid")
		return
	}

	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		h.handleResponse(c, status_http.BadRequest, "error getting environment id | not valid")
		return
	}

	userId, _ := c.Get("user_id")
	apiKey := c.GetHeader("X-API-KEY")

	resource, err := h.companyServices.ServiceResource().GetSingle(
		c.Request.Context(),
		&pb.GetSingleServiceResourceReq{
			ProjectId:     projectId.(string),
			EnvironmentId: environmentId.(string),
			ServiceType:   pb.ServiceType_BUILDER_SERVICE,
		},
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	services, err := h.GetProjectSrvc(c.Request.Context(), projectId.(string), resource.NodeType)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	var (
		logReq = &models.CreateVersionHistoryRequest{
			Services:     services,
			NodeType:     resource.NodeType,
			ProjectId:    resource.ResourceEnvironmentId,
			ActionSource: c.Request.URL.String(),
			ActionType:   "GET",
			UserInfo:     cast.ToString(userId),
			Request:      &structData,
			ApiKey:       apiKey,
			Type:         "API_KEY",
			TableSlug:    c.Param("collection"),
		}
	)

	switch resource.ResourceType {
	case pb.ResourceType_MONGODB:
		service := services.GetBuilderServiceByType(resource.NodeType).ObjectBuilder()
		redisResp, err := h.redis.Get(context.Background(), base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s-%s-%s", c.Param("collection"), structData.String(), resource.ResourceEnvironmentId))), projectId.(string), resource.NodeType)
		if err == nil {
			var (
				resp = make(map[string]interface{})
				m    = make(map[string]interface{})
			)

			if err = json.Unmarshal([]byte(redisResp), &m); err != nil {
				h.log.Error("Error while unmarshal redis", logger.Error(err))
			} else {
				resp["data"] = m
				h.handleResponse(c, status_http.OK, resp)
				logReq.Response = m
				go h.versionHistory(logReq)
				return
			}
		} else {
			h.log.Error("Error while getting redis", logger.Error(err))
		}

		resp, err := service.GetSingleSlim(
			c.Request.Context(),
			&obs.CommonMessage{
				TableSlug: c.Param("collection"),
				Data:      structData,
				ProjectId: resource.ResourceEnvironmentId,
			},
		)
		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			logReq.Response = err.Error()
			go h.versionHistory(logReq)
			h.handleResponse(c, statusHttp, err.Error())
			return
		}

		logReq.Response = resp
		go h.versionHistory(logReq)

		if resp.IsCached {
			jsonData, _ := resp.GetData().MarshalJSON()
			err = h.redis.SetX(context.Background(), base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s-%s-%s", c.Param("collection"), structData.String(), resource.ResourceEnvironmentId))), string(jsonData), 15*time.Second, projectId.(string), resource.NodeType)
			if err != nil {
				h.log.Error("Error while setting redis", logger.Error(err))
			}
		}

		statusHttp.CustomMessage = resp.GetCustomMessage()
		if hashed {
			response, err := security.Encrypt(resp, h.baseConf.SecretKey)
			if err != nil {
				h.handleResponse(c, status_http.InternalServerError, err.Error())
				return
			}
			h.handleResponse(c, statusHttp, response)
			return
		}
		h.handleResponse(c, statusHttp, resp)
	case pb.ResourceType_POSTGRESQL:
		redisResp, err := h.redis.Get(context.Background(), base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s-%s-%s", c.Param("collection"), structData.String(), resource.ResourceEnvironmentId))), projectId.(string), resource.NodeType)
		if err == nil {
			var (
				resp = make(map[string]interface{})
				m    = make(map[string]interface{})
			)

			if err = json.Unmarshal([]byte(redisResp), &m); err != nil {
				h.log.Error("Error while unmarshal redis", logger.Error(err))
			} else {
				resp["data"] = m
				h.handleResponse(c, status_http.OK, resp)
				logReq.Response = m
				go h.versionHistory(logReq)
				return
			}
		} else {
			h.log.Error("Error while getting redis", logger.Error(err))
		}

		resp, err := services.GoObjectBuilderService().ObjectBuilder().GetSingleSlim(
			c.Request.Context(), &nb.CommonMessage{
				TableSlug: c.Param("collection"),
				Data:      structData,
				ProjectId: resource.ResourceEnvironmentId,
			},
		)

		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			logReq.Response = err.Error()
			go h.versionHistoryGo(c, logReq)
			h.handleResponse(c, statusHttp, err.Error())
			return
		}

		logReq.Response = resp
		go h.versionHistoryGo(c, logReq)

		if resp.IsCached {
			jsonData, _ := resp.GetData().MarshalJSON()
			err = h.redis.SetX(context.Background(), base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s-%s-%s", c.Param("collection"), structData.String(), resource.ResourceEnvironmentId))), string(jsonData), 15*time.Second, projectId.(string), resource.NodeType)
			if err != nil {
				h.log.Error("Error while setting redis", logger.Error(err))
			}
		}

		statusHttp.CustomMessage = resp.GetCustomMessage()
		h.handleResponse(c, statusHttp, resp)
	}
}

// CreateItem godoc
// @Security ApiKeyAuth
// @ID create_item
// @Router /v2/items/{collection} [POST]
// @Summary Create item
// @Description Create item
// @Tags Items
// @Accept json
// @Produce json
// @Param collection path string true "collection"
// @Param object body models.CommonMessage true "CreateItemsRequestBody"
// @Success 201 {object} status_http.Response{data=models.CommonMessage} "Object data"
// @Response 400 {object} status_http.Response{data=string} "Bad Request"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) CreateItem(c *gin.Context) {
	var (
		objectRequest               models.CommonMessage
		resp                        *obs.CommonMessage
		beforeActions, afterActions []*obs.CustomEvent
		statusHttp                  = status_http.GrpcStatusToHTTP["Created"]
	)

	err := c.ShouldBindJSON(&objectRequest)
	if err != nil {
		h.handleResponse(c, status_http.BadRequest, err.Error())
		return
	}

	projectId, ok := c.Get("project_id")
	if !ok || !util.IsValidUUID(projectId.(string)) {
		h.handleResponse(c, status_http.InvalidArgument, "project id is an invalid uuid")
		return
	}

	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		err = errors.New("error getting environment id | not valid")
		h.handleResponse(c, status_http.BadRequest, err)
		return
	}

	userId, _ := c.Get("user_id")

	resource, err := h.companyServices.ServiceResource().GetSingle(
		c.Request.Context(),
		&pb.GetSingleServiceResourceReq{
			ProjectId:     projectId.(string),
			EnvironmentId: environmentId.(string),
			ServiceType:   pb.ServiceType_BUILDER_SERVICE,
		},
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	services, err := h.GetProjectSrvc(c.Request.Context(),
		projectId.(string),
		resource.NodeType,
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	objectRequest.Data["company_service_project_id"] = resource.GetProjectId()
	objectRequest.Data["company_service_environment_id"] = resource.GetEnvironmentId()

	var id string
	uid, _ := uuid.NewRandom()
	id = uid.String()

	guid, ok := objectRequest.Data["guid"]
	if ok {
		if util.IsValidUUID(guid.(string)) {
			id = objectRequest.Data["guid"].(string)
		}
	}

	objectRequest.Data["guid"] = id

	structData, err := helper.ConvertMapToStruct(objectRequest.Data)

	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}
	fromOfs := c.Query("from-ofs")
	if fromOfs != "true" {
		beforeActions, afterActions, err = GetListCustomEvents(c.Param("collection"), "", "CREATE", c, h)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error())
			//return 
		}
	}
	if len(beforeActions) > 0 {
		functionName, err := DoInvokeFuntion(DoInvokeFuntionStruct{
			CustomEvents: beforeActions,
			IDs:          []string{id},
			TableSlug:    c.Param("collection"),
			ObjectData:   objectRequest.Data,
			Method:       "CREATE",
		},
			c,
			h,
		)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error()+" in "+functionName)
			return
		}
	}

	logReq := &models.CreateVersionHistoryRequest{
		Services:     services,
		NodeType:     resource.NodeType,
		ProjectId:    resource.ResourceEnvironmentId,
		ActionSource: c.Request.URL.String(),
		ActionType:   "CREATE ITEM",
		UsedEnvironments: map[string]bool{
			cast.ToString(environmentId): true,
		},
		UserInfo:  cast.ToString(userId),
		Request:   &structData,
		TableSlug: c.Param("collection"),
	}

	switch resource.ResourceType {
	case pb.ResourceType_MONGODB:
		resp, err = services.GetBuilderServiceByType(resource.NodeType).ItemsService().Create(
			context.Background(),
			&obs.CommonMessage{
				TableSlug: c.Param("collection"),
				Data:      structData,
				ProjectId: resource.ResourceEnvironmentId,
			},
		)
		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			logReq.Response = err.Error()
			defer func() { go h.versionHistory(logReq) }()
			h.handleResponse(c, statusHttp, err.Error())
			return
		}
		logReq.Response = resp
		defer func() { go h.versionHistory(logReq) }()
	case pb.ResourceType_POSTGRESQL:
		body, err := services.GoObjectBuilderService().Items().Create(
			context.Background(),
			&nb.CommonMessage{
				TableSlug: c.Param("collection"),
				Data:      structData,
				ProjectId: resource.ResourceEnvironmentId,
			},
		)
		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			logReq.Response = err.Error()
			defer func() { go h.versionHistoryGo(c, logReq) }()
			h.handleResponse(c, statusHttp, err.Error())
			return
		}

		err = helper.MarshalToStruct(body, &resp)
		if err != nil {
			return
		}

		logReq.Response = resp
		defer func() { go h.versionHistoryGo(c, logReq) }()
	}

	if data, ok := resp.Data.AsMap()["data"].(map[string]interface{}); ok {
		objectRequest.Data = data
		if _, ok = data["guid"].(string); ok {
			id = data["guid"].(string)
		}
	}
	if len(afterActions) > 0 {
		functionName, err := DoInvokeFuntion(
			DoInvokeFuntionStruct{
				CustomEvents: afterActions,
				IDs:          []string{id},
				TableSlug:    c.Param("collection"),
				ObjectData:   objectRequest.Data,
				Method:       "CREATE",
			},
			c, // gin context,
			h, // handler
		)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error()+" in "+functionName)
			return
		}
	}
	statusHttp.CustomMessage = resp.GetCustomMessage()
	h.handleResponse(c, statusHttp, resp)
}

// CreateItems godoc
// @Security ApiKeyAuth
// @ID create_items
// @Router /v2/items/{collection}/multiple-insert [POST]
// @Summary Create items
// @Description Create items
// @Tags Items
// @Accept json
// @Produce json
// @Param collection path string true "collection"
// @Param object body models.MultipleInsertItems true "CreateItemsRequestBody"
// @Success 201 {object} status_http.Response{data=models.CommonMessage} "Object data"
// @Response 400 {object} status_http.Response{data=string} "Bad Request"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) CreateItems(c *gin.Context) {
	var (
		objectRequest               models.MultipleInsertItems
		resp                        *obs.CommonMessage
		beforeActions, afterActions []*obs.CustomEvent
		statusHttp                  = status_http.GrpcStatusToHTTP["Created"]
	)

	err := c.ShouldBindJSON(&objectRequest)
	if err != nil {
		h.handleResponse(c, status_http.BadRequest, err.Error())
		return
	}

	projectId, ok := c.Get("project_id")
	if !ok || !util.IsValidUUID(projectId.(string)) {
		h.handleResponse(c, status_http.InvalidArgument, "project id is an invalid uuid")
		return
	}

	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		err = errors.New("error getting environment id | not valid")
		h.handleResponse(c, status_http.BadRequest, err)
		return
	}

	userId, _ := c.Get("user_id")

	resource, err := h.companyServices.ServiceResource().GetSingle(
		c.Request.Context(),
		&pb.GetSingleServiceResourceReq{
			ProjectId:     projectId.(string),
			EnvironmentId: environmentId.(string),
			ServiceType:   pb.ServiceType_BUILDER_SERVICE,
		},
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	services, err := h.GetProjectSrvc(
		c.Request.Context(),
		resource.GetProjectId(),
		resource.NodeType,
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	request := make(map[string]interface{})
	request["company_service_project_id"] = resource.GetProjectId()
	request["company_service_environment_id"] = resource.GetEnvironmentId()
	request["items"] = objectRequest

	structData, err := helper.ConvertMapToStruct(request)

	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}
	fromOfs := c.Query("from-ofs")
	if fromOfs != "true" {
		beforeActions, afterActions, err = GetListCustomEvents(c.Param("collection"), "", "CREATE_MANY", c, h)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error())
			return
		}
	}
	if len(beforeActions) > 0 {
		functionName, err := DoInvokeFuntion(DoInvokeFuntionStruct{
			CustomEvents: beforeActions,
			TableSlug:    c.Param("collection"),
			ObjectData:   request,
			Method:       "CREATE_MANY",
		},
			c,
			h,
		)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error()+" in "+functionName)
			return
		}
	}

	logReq := &models.CreateVersionHistoryRequest{
		Services:     services,
		NodeType:     resource.NodeType,
		ProjectId:    resource.ResourceEnvironmentId,
		ActionSource: c.Request.URL.String(),
		ActionType:   "CREATE ITEM",
		UsedEnvironments: map[string]bool{
			cast.ToString(environmentId): true,
		},
		UserInfo:  cast.ToString(userId),
		Request:   structData,
		TableSlug: c.Param("collection"),
	}

	switch resource.ResourceType {
	case pb.ResourceType_MONGODB:
		resp, err = services.GetBuilderServiceByType(resource.NodeType).ObjectBuilder().Create(
			context.Background(),
			&obs.CommonMessage{
				TableSlug: c.Param("collection"),
				Data:      structData,
				ProjectId: resource.ResourceEnvironmentId,
			},
		)
		// this logic for custom error message, object builder service may be return 400, 404, 500
		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			logReq.Response = err.Error()
			go h.versionHistory(logReq)
			h.handleResponse(c, statusHttp, err.Error())
			return
		}
		logReq.Response = resp
		go h.versionHistory(logReq)
	case pb.ResourceType_POSTGRESQL:
	}
	var items []interface{}
	if itemsFromResp, ok := resp.Data.AsMap()["items"].([]interface{}); ok {
		items = itemsFromResp
	}
	var ids = make([]string, 0, len(items))
	for _, item := range items {
		if itemMap, ok := item.(map[string]interface{}); ok {
			if id, ok := itemMap["guid"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	if len(afterActions) > 0 {
		functionName, err := DoInvokeFuntion(
			DoInvokeFuntionStruct{
				CustomEvents: afterActions,
				IDs:          ids,
				TableSlug:    c.Param("collection"),
				ObjectData:   request,
				Method:       "CREATE_MANY",
			},
			c, // gin context,
			h, // handler
		)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error()+" in "+functionName)
			return
		}
	}
	statusHttp.CustomMessage = resp.GetCustomMessage()
	h.handleResponse(c, statusHttp, resp)
}

// UpdateItem godoc
// @Security ApiKeyAuth
// @ID update_item
// @Router /v2/items/{collection} [PUT]
// @Summary Update item
// @Description Update item
// @Tags Items
// @Accept json
// @Produce json
// @Param collection path string true "collection"
// @Param item body models.CommonMessage true "UpdateItemRequestBody"
// @Success 200 {object} status_http.Response{data=models.CommonMessage} "Item data"
// @Response 400 {object} status_http.Response{data=string} "Bad Request"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) UpdateItem(c *gin.Context) {
	var (
		objectRequest               models.CommonMessage
		resp, singleObject          *obs.CommonMessage
		beforeActions, afterActions []*obs.CustomEvent
		statusHttp                  = status_http.GrpcStatusToHTTP["Ok"]
		actionErr                   error
		functionName                string
		id                          string
	)

	err := c.ShouldBindJSON(&objectRequest)
	if err != nil {
		h.handleResponse(c, status_http.BadRequest, err.Error())
		return
	}

	structData, err := helper.ConvertMapToStruct(objectRequest.Data)

	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}
	if objectRequest.Data["guid"] != nil {
		id = objectRequest.Data["guid"].(string)
	} else {
		objectRequest.Data["guid"] = c.Param("id")
		id = c.Param("id")

		if id == "" {
			h.handleResponse(c, status_http.BadRequest, "guid is required")
			return
		}
	}

	projectId, ok := c.Get("project_id")
	if !ok || !util.IsValidUUID(projectId.(string)) {
		h.handleResponse(c, status_http.InvalidArgument, "project id is an invalid uuid")
		return
	}

	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		err = errors.New("error getting environment id | not valid")
		h.handleResponse(c, status_http.BadRequest, err)
		return
	}

	userId, _ := c.Get("user_id")

	resource, err := h.companyServices.ServiceResource().GetSingle(
		c.Request.Context(),
		&pb.GetSingleServiceResourceReq{
			ProjectId:     projectId.(string),
			EnvironmentId: environmentId.(string),
			ServiceType:   pb.ServiceType_BUILDER_SERVICE,
		},
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	services, err := h.GetProjectSrvc(
		c.Request.Context(),
		resource.GetProjectId(),
		resource.NodeType,
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	switch resource.ResourceType {
	case pb.ResourceType_MONGODB:
		singleObject, err = services.GetBuilderServiceByType(resource.NodeType).ObjectBuilder().GetSingleSlim(
			context.Background(),
			&obs.CommonMessage{
				TableSlug: c.Param("collection"),
				Data: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"id": structpb.NewStringValue(id),
					},
				},
				ProjectId: resource.ResourceEnvironmentId,
			},
		)
		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			h.handleResponse(c, statusHttp, err.Error())
			return
		}
	case pb.ResourceType_POSTGRESQL:
		single, err := services.GoObjectBuilderService().Items().GetSingle(
			context.Background(),
			&nb.CommonMessage{
				TableSlug: c.Param("collection"),
				Data: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"id": structpb.NewStringValue(id),
					},
				},
				ProjectId: resource.ResourceEnvironmentId,
			},
		)
		if err != nil {
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}

		err = helper.MarshalToStruct(single, &singleObject)
		if err != nil {
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}
	}

	fromOfs := c.Query("from-ofs")
	if fromOfs != "true" {
		beforeActions, afterActions, err = GetListCustomEvents(c.Param("collection"), "", "UPDATE", c, h)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error())
			return
		}
	}
	if len(beforeActions) > 0 {
		functionName, err := DoInvokeFuntion(DoInvokeFuntionStruct{
			CustomEvents: beforeActions,
			IDs:          []string{id},
			TableSlug:    c.Param("collection"),
			ObjectData:   objectRequest.Data,
			Method:       "UPDATE",
			ActionType:   "BEFORE",
		},
			c,
			h,
		)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error()+" in "+functionName)
			return
		}
	}

	var (
		logReq = &models.CreateVersionHistoryRequest{
			Services:     services,
			NodeType:     resource.NodeType,
			ProjectId:    resource.ResourceEnvironmentId,
			ActionSource: c.Request.URL.String(),
			ActionType:   "UPDATE ITEM",
			UsedEnvironments: map[string]bool{
				cast.ToString(environmentId): true,
			},
			UserInfo:  cast.ToString(userId),
			Request:   &structData,
			TableSlug: c.Param("collection"),
		}
	)

	defer func() {
		if err != nil {
			logReq.Response = err.Error()
			h.handleResponse(c, status_http.GRPCError, err.Error())
		} else if actionErr != nil {
			logReq.Response = actionErr.Error() + " in " + functionName
			h.handleResponse(c, status_http.InvalidArgument, actionErr.Error()+" in "+functionName)
		} else {
			logReq.Response = resp
			h.handleResponse(c, status_http.OK, resp)
		}
		switch resource.ResourceType {
		case pb.ResourceType_MONGODB:
			go h.versionHistory(logReq)
		case pb.ResourceType_POSTGRESQL:
			go h.versionHistoryGo(c, logReq)
		}
	}()

	switch resource.ResourceType {
	case pb.ResourceType_MONGODB:
		resp, err = services.GetBuilderServiceByType(resource.NodeType).ObjectBuilder().Update(
			context.Background(),
			&obs.CommonMessage{
				TableSlug:        c.Param("collection"),
				Data:             structData,
				ProjectId:        resource.ResourceEnvironmentId,
				EnvId:            resource.EnvironmentId,
				CompanyProjectId: resource.ProjectId,
			},
		)
		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			return
		}
	case pb.ResourceType_POSTGRESQL:
		body, err := services.GoObjectBuilderService().Items().Update(
			context.Background(),
			&nb.CommonMessage{
				TableSlug:        c.Param("collection"),
				Data:             structData,
				ProjectId:        resource.ResourceEnvironmentId,
				BlockedBuilder:   cast.ToBool(c.DefaultQuery("block_builder", "false")),
				EnvId:            resource.EnvironmentId,
				CompanyProjectId: resource.ProjectId,
			},
		)
		if err != nil {
			return
		}

		err = helper.MarshalToStruct(body, &resp)
		if err != nil {
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}
	}
	if len(afterActions) > 0 {
		// h.log.Info("---ObjectBefore--->>", logger.Any("body", singleObject.Data.AsMap()))
		functionName, actionErr = DoInvokeFuntion(
			DoInvokeFuntionStruct{
				CustomEvents:           afterActions,
				IDs:                    []string{id},
				TableSlug:              c.Param("collection"),
				ObjectData:             objectRequest.Data,
				Method:                 "UPDATE",
				ObjectDataBeforeUpdate: singleObject.Data.AsMap(),
				ActionType:             "AFTER",
			},
			c, // gin context,
			h, // handler
		)
		if err != nil {
			return
		}
	}
	statusHttp.CustomMessage = resp.GetCustomMessage()
}

// MultipleUpdateItems godoc
// @Security ApiKeyAuth
// @ID multiple_update_items
// @Router /v2/items/{collection} [PATCH]
// @Summary Multiple Update items
// @Description Multiple Update items
// @Tags Items
// @Accept json
// @Produce json
// @Param collection path string true "collection"
// @Param items body models.MultipleUpdateItems true "MultipleItemsRequesUpdatetBody"
// @Success 201 {object} status_http.Response{data=models.CommonMessage} "Items data"
// @Response 400 {object} status_http.Response{data=string} "Bad Request"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) MultipleUpdateItems(c *gin.Context) {
	var (
		objectRequest               models.CommonMessage
		beforeActions, afterActions []*obs.CustomEvent
		statusHttp                  = status_http.GrpcStatusToHTTP["Created"]
		resource                    *pb.ServiceResourceModel
		resourceList                *pb.GetResourceByEnvIDResponse
		resp                        *obs.CommonMessage
		err                         error
	)

	if err = c.ShouldBindJSON(&objectRequest); err != nil {
		h.handleResponse(c, status_http.BadRequest, err.Error())
		return
	}

	projectId, ok := c.Get("project_id")
	if !ok || !util.IsValidUUID(projectId.(string)) {
		h.handleResponse(c, status_http.InvalidArgument, "project id is an invalid uuid")
		return
	}

	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		h.handleResponse(c, status_http.BadRequest, "error getting environment id | not valid")
		return
	}

	if resourceBody, ok := c.Get("resource"); ok {
		if err = json.Unmarshal([]byte(resourceBody.(string)), &resourceList); err != nil {
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}

		for _, resourceObject := range resourceList.ServiceResources {
			if resourceObject.Title == pb.ServiceType_name[1] {
				resource = &pb.ServiceResourceModel{
					Id:                    resourceObject.Id,
					ServiceType:           resourceObject.ServiceType,
					ProjectId:             resourceObject.ProjectId,
					Title:                 resourceObject.Title,
					ResourceId:            resourceObject.ResourceId,
					ResourceEnvironmentId: resourceObject.ResourceEnvironmentId,
					EnvironmentId:         resourceObject.EnvironmentId,
					ResourceType:          resourceObject.ResourceType,
					NodeType:              resourceObject.NodeType,
				}
				break
			}
		}
	} else {
		resource, err = h.companyServices.ServiceResource().GetSingle(c.Request.Context(),
			&pb.GetSingleServiceResourceReq{
				ProjectId:     projectId.(string),
				EnvironmentId: environmentId.(string),
				ServiceType:   pb.ServiceType_BUILDER_SERVICE,
			},
		)
		if err != nil {
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}
	}

	services, err := h.GetProjectSrvc(c.Request.Context(), projectId.(string), resource.NodeType)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	service := services.GetBuilderServiceByType(resource.NodeType).ObjectBuilder()

	objects, ok := objectRequest.Data["objects"].([]interface{})
	if !ok {
		err = errors.New("objects is not an array")
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	editedObjects := make([]map[string]interface{}, 0, len(objects))
	objectIds := make([]string, 0, len(objects))

	for _, object := range objects {
		newObjects := object.(map[string]interface{})

		if _, ok := newObjects["guid"].(string); !ok {
			newObjects["guid"] = uuid.NewString()
			newObjects["is_new"] = true
		}

		newObjects["company_service_project_id"] = resource.GetProjectId()
		newObjects["company_service_environment_id"] = resource.GetEnvironmentId()

		objectIds = append(objectIds, newObjects["guid"].(string))
		editedObjects = append(editedObjects, newObjects)
	}

	structData, err := helper.ConvertMapToStruct(objectRequest.Data)
	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}
	objectRequest.Data["objects"] = editedObjects

	fromOfs := c.Query("from-ofs")
	if fromOfs != "true" {
		beforeActions, afterActions, err = GetListCustomEvents(c.Param("collection"), "", "MULTIPLE_UPDATE", c, h)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error())
			return
		}
	}

	if len(beforeActions) > 0 {
		functionName, err := DoInvokeFuntion(DoInvokeFuntionStruct{
			CustomEvents: beforeActions,
			IDs:          objectIds,
			TableSlug:    c.Param("collection"),
			ObjectData:   objectRequest.Data,
			Method:       "MULTIPLE_UPDATE",
			Resource:     resource,
		}, c, h)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error()+" in "+functionName)
			return
		}
	}

	switch resource.ResourceType {
	case pb.ResourceType_MONGODB:
		resp, err = service.MultipleUpdate(
			c.Request.Context(), &obs.CommonMessage{
				TableSlug:        c.Param("collection"),
				Data:             structData,
				ProjectId:        resource.ResourceEnvironmentId,
				BlockedBuilder:   cast.ToBool(c.DefaultQuery("block_builder", "false")),
				EnvId:            resource.EnvironmentId,
				CompanyProjectId: resource.ProjectId,
			},
		)
		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			if stat, ok := status.FromError(err); ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			h.handleResponse(c, statusHttp, err.Error())
			return
		}
	case pb.ResourceType_POSTGRESQL:
		goResp, err := services.GoObjectBuilderService().Items().MultipleUpdate(
			c.Request.Context(), &nb.CommonMessage{
				TableSlug:        c.Param("collection"),
				Data:             structData,
				ProjectId:        resource.ResourceEnvironmentId,
				EnvId:            resource.EnvironmentId,
				CompanyProjectId: resource.ProjectId,
			},
		)

		if err != nil {
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}

		if err = helper.MarshalToStruct(goResp, &resp); err != nil {
			h.handleResponse(c, status_http.BadRequest, err.Error())
			return
		}
	}

	if len(afterActions) > 0 {
		functionName, err := DoInvokeFuntion(DoInvokeFuntionStruct{
			CustomEvents: afterActions,
			IDs:          objectIds,
			TableSlug:    c.Param("collection"),
			ObjectData:   objectRequest.Data,
			Method:       "MULTIPLE_UPDATE",
			Resource:     resource,
		},
			c,
			h,
		)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error()+" in "+functionName)
			return
		}
	}

	statusHttp.CustomMessage = resp.GetCustomMessage()
	h.handleResponse(c, status_http.Created, resp)
}

// DeleteItem godoc
// @Security ApiKeyAuth
// @ID delete_item
// @Router /v2/items/{collection}/{id} [DELETE]
// @Summary Delete item
// @Description Delete item
// @Tags Items
// @Accept json
// @Produce json
// @Param collection path string true "collection"
// @Param id path string true "id"
// @Success 204
// @Response 400 {object} status_http.Response{data=string} "Invalid Argument"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) DeleteItem(c *gin.Context) {
	var (
		objectRequest               models.CommonMessage
		resp                        *obs.CommonMessage
		beforeActions, afterActions []*obs.CustomEvent
		statusHttp                  = status_http.GrpcStatusToHTTP["NoContent"]
	)

	err := c.ShouldBindJSON(&objectRequest)
	if err != nil {
		h.handleResponse(c, status_http.BadRequest, err.Error())
		return
	}

	objectID := c.Param("id")
	if !util.IsValidUUID(objectID) {
		h.handleResponse(c, status_http.InvalidArgument, "item id is an invalid uuid")
		return
	}

	projectId, ok := c.Get("project_id")
	if !ok || !util.IsValidUUID(projectId.(string)) {
		h.handleResponse(c, status_http.InvalidArgument, "project id is an invalid uuid")
		return
	}

	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		err = errors.New("error getting environment id | not valid")
		h.handleResponse(c, status_http.BadRequest, err)
		return
	}

	userId, _ := c.Get("user_id")

	resource, err := h.companyServices.ServiceResource().GetSingle(
		c.Request.Context(),
		&pb.GetSingleServiceResourceReq{
			ProjectId:     projectId.(string),
			EnvironmentId: environmentId.(string),
			ServiceType:   pb.ServiceType_BUILDER_SERVICE,
		},
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	services, err := h.GetProjectSrvc(
		c.Request.Context(),
		resource.GetProjectId(),
		resource.NodeType,
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	objectRequest.Data["id"] = objectID
	objectRequest.Data["company_service_project_id"] = projectId.(string)
	objectRequest.Data["company_service_environment_id"] = environmentId.(string)

	structData, err := helper.ConvertMapToStruct(objectRequest.Data)
	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}

	fromOfs := c.Query("from-ofs")
	if fromOfs != "true" {
		beforeActions, afterActions, err = GetListCustomEvents(c.Param("collection"), "", "DELETE", c, h)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error())
			return
		}
	}
	if len(beforeActions) > 0 {
		functionName, err := DoInvokeFuntion(DoInvokeFuntionStruct{
			CustomEvents: beforeActions,
			IDs:          []string{objectID},
			TableSlug:    c.Param("collection"),
			ObjectData:   objectRequest.Data,
			Method:       "DELETE",
		},
			c,
			h,
		)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error()+" in "+functionName)
			return
		}
	}

	logReq := &models.CreateVersionHistoryRequest{
		Services:     services,
		NodeType:     resource.NodeType,
		ProjectId:    resource.ResourceEnvironmentId,
		ActionSource: c.Request.URL.String(),
		ActionType:   "DELETE ITEM",
		UsedEnvironments: map[string]bool{
			cast.ToString(environmentId): true,
		},
		UserInfo:  cast.ToString(userId),
		Request:   structData,
		TableSlug: c.Param("collection"),
	}

	switch resource.ResourceType {
	case pb.ResourceType_MONGODB:
		resp, err = services.GetBuilderServiceByType(resource.NodeType).ObjectBuilder().Delete(
			context.Background(),
			&obs.CommonMessage{
				TableSlug: c.Param("collection"),
				Data:      structData,
				ProjectId: resource.ResourceEnvironmentId,
			},
		)
		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			logReq.Response = err.Error()
			go h.versionHistory(logReq)
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}
		logReq.Response = resp
		go h.versionHistory(logReq)
	case pb.ResourceType_POSTGRESQL:
		new, err := services.GoObjectBuilderService().Items().Delete(
			context.Background(),
			&nb.CommonMessage{
				TableSlug: c.Param("collection"),
				Data:      structData,
				ProjectId: resource.ResourceEnvironmentId,
			},
		)
		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			logReq.Response = err.Error()
			go h.versionHistoryGo(c, logReq)
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}
		logReq.Response = resp
		go h.versionHistoryGo(c, logReq)

		err = helper.MarshalToStruct(new, &resp)
		if err != nil {
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}

	}

	if len(afterActions) > 0 {
		functionName, err := DoInvokeFuntion(
			DoInvokeFuntionStruct{
				CustomEvents: afterActions,
				IDs:          []string{objectID},
				TableSlug:    c.Param("collection"),
				ObjectData:   objectRequest.Data,
				Method:       "DELETE",
			},
			c, // gin context,
			h, // handler
		)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error()+" in "+functionName)
			return
		}
	}

	statusHttp.CustomMessage = resp.GetCustomMessage()
	h.handleResponse(c, statusHttp, resp)
}

// DeleteManyObject godoc
// @Security ApiKeyAuth
// @ID delete_items
// @Router /v2/items/{collection} [DELETE]
// @Summary Delete many itmes
// @Description Delete many itmes
// @Tags Items
// @Accept json
// @Produce json
// @Param collection path string true "collection"
// @Param object body models.Ids true "DeleteManyItemRequestBody"
// @Success 204
// @Response 400 {object} status_http.Response{data=string} "Invalid Argument"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) DeleteItems(c *gin.Context) {
	var (
		objectRequest               models.Ids
		resp                        *obs.CommonMessage
		beforeActions, afterActions []*obs.CustomEvent
		statusHttp                  = status_http.GrpcStatusToHTTP["NoContent"]
		data                        = make(map[string]interface{})
		actionErr                   error
		functionName                string
	)

	err := c.ShouldBindJSON(&objectRequest)
	if err != nil {
		h.handleResponse(c, status_http.BadRequest, err.Error())
		return
	}

	projectId, ok := c.Get("project_id")
	if !ok || !util.IsValidUUID(projectId.(string)) {
		h.handleResponse(c, status_http.InvalidArgument, "project id is an invalid uuid")
		return
	}

	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		err = errors.New("error getting environment id | not valid")
		h.handleResponse(c, status_http.BadRequest, err)
		return
	}

	userId, _ := c.Get("user_id")

	resource, err := h.companyServices.ServiceResource().GetSingle(
		c.Request.Context(),
		&pb.GetSingleServiceResourceReq{
			ProjectId:     projectId.(string),
			EnvironmentId: environmentId.(string),
			ServiceType:   pb.ServiceType_BUILDER_SERVICE,
		},
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	data["company_service_project_id"] = projectId.(string)
	data["company_service_environment_id"] = environmentId.(string)
	data["ids"] = objectRequest.Ids

	services, err := h.GetProjectSrvc(
		c.Request.Context(),
		resource.GetProjectId(),
		resource.NodeType,
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	service := services.GetBuilderServiceByType(resource.NodeType).ObjectBuilder()
	if err != nil {
		h.log.Info("Error while getting "+resource.NodeType+" object builder service", logger.Error(err))
		h.handleResponse(c, status_http.InternalServerError, err)
		return
	}

	structData, err := helper.ConvertMapToStruct(data)
	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}

	fromOfs := c.Query("from-ofs")
	if fromOfs != "true" {
		beforeActions, afterActions, err = GetListCustomEvents(c.Param("collection"), "", "DELETE_MANY", c, h)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error())
			return
		}
	}
	if len(beforeActions) > 0 {
		functionName, err := DoInvokeFuntion(DoInvokeFuntionStruct{
			CustomEvents: beforeActions,
			IDs:          objectRequest.Ids,
			TableSlug:    c.Param("collection"),
			ObjectData:   data,
			Method:       "DELETE_MANY",
		},
			c,
			h,
		)
		if err != nil {
			h.handleResponse(c, status_http.InvalidArgument, err.Error()+" in "+functionName)
			return
		}
	}

	var (
		logReq = &models.CreateVersionHistoryRequest{
			Services:     services,
			NodeType:     resource.NodeType,
			ProjectId:    resource.ResourceEnvironmentId,
			ActionSource: c.Request.URL.String(),
			ActionType:   "DELETE ITEM",
			UsedEnvironments: map[string]bool{
				cast.ToString(environmentId): true,
			},
			UserInfo:  cast.ToString(userId),
			Request:   &structData,
			TableSlug: c.Param("collection"),
		}
	)

	defer func() {
		if err != nil {
			logReq.Response = err.Error()
			h.handleResponse(c, status_http.GRPCError, err.Error())
		} else if actionErr != nil {
			logReq.Response = actionErr.Error() + " in " + functionName
			h.handleResponse(c, status_http.InvalidArgument, actionErr.Error()+" in "+functionName)
		} else {
			logReq.Response = resp
			h.handleResponse(c, status_http.NoContent, resp)
		}
		go h.versionHistory(logReq)
	}()

	switch resource.ResourceType {
	case pb.ResourceType_MONGODB:
		resp, err = service.DeleteMany(
			context.Background(),
			&obs.CommonMessage{
				TableSlug: c.Param("collection"),
				Data:      structData,
				ProjectId: resource.ResourceEnvironmentId,
			},
		)
		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}
	case pb.ResourceType_POSTGRESQL:
		_, err = services.GoObjectBuilderService().Items().DeleteMany(
			c.Request.Context(), &nb.CommonMessage{
				TableSlug: c.Param("collection"),
				Data:      structData,
				ProjectId: resource.ResourceEnvironmentId,
			},
		)

		if err != nil {
			statusHttp = status_http.GrpcStatusToHTTP["Internal"]
			stat, ok := status.FromError(err)
			if ok {
				statusHttp = status_http.GrpcStatusToHTTP[stat.Code().String()]
				statusHttp.CustomMessage = stat.Message()
			}
			h.handleResponse(c, status_http.GRPCError, err.Error())
			return
		}
	}

	if len(afterActions) > 0 {
		functionName, actionErr = DoInvokeFuntion(
			DoInvokeFuntionStruct{
				CustomEvents: afterActions,
				IDs:          objectRequest.Ids,
				TableSlug:    c.Param("collection"),
				ObjectData:   data,
				Method:       "DELETE_MANY",
			},
			c, // gin context,
			h, // handler
		)
		if err != nil {
			return
		}
	}

	statusHttp.CustomMessage = resp.GetCustomMessage()
}

// GetListAggregation godoc
// @Security ApiKeyAuth
// @ID get_list_aggregation
// @Router /v2/items/{collection}/aggregation [POST]
// @Summary Get List Aggregation
// @Description Get List Aggregation
// @Tags Items
// @Accept json
// @Produce json
// @Param collection path string true "collection"
// @Param object body models.CommonMessage true "GetListAggregation"
// @Success 200 {object} status_http.Response{data=models.CommonMessage} "ObjectBody"
// @Response 400 {object} status_http.Response{data=string} "Invalid Argument"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) GetListAggregation(c *gin.Context) {

	var (
		reqBody models.CommonMessage
	)

	err := c.ShouldBindJSON(&reqBody)
	if err != nil {
		h.handleResponse(c, status_http.BadRequest, err.Error())
	}

	key, err := json.Marshal(reqBody.Data)
	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}

	structData, err := helper.ConvertMapToStruct(reqBody.Data)
	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}

	projectId, ok := c.Get("project_id")
	if !ok || !util.IsValidUUID(projectId.(string)) {
		h.handleResponse(c, status_http.InvalidArgument, "project id is an invalid uuid")
		return
	}

	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		err = errors.New("error getting environment id | not valid")
		h.handleResponse(c, status_http.BadRequest, err)
		return
	}

	resource, err := h.companyServices.ServiceResource().GetSingle(
		c.Request.Context(),
		&pb.GetSingleServiceResourceReq{
			ProjectId:     projectId.(string),
			EnvironmentId: environmentId.(string),
			ServiceType:   pb.ServiceType_BUILDER_SERVICE,
		},
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	services, err := h.GetProjectSrvc(
		c.Request.Context(),
		projectId.(string),
		resource.NodeType,
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	service := services.GetBuilderServiceByType(resource.NodeType).ObjectBuilder()

	if reqBody.IsCached {
		redisResp, err := h.redis.Get(context.Background(), base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s-%s-%s", c.Param("collection"), string(key), resource.ResourceEnvironmentId))), projectId.(string), resource.NodeType)
		if err == nil {
			resp := make(map[string]interface{})
			m := make(map[string]interface{})
			err = json.Unmarshal([]byte(redisResp), &m)
			if err != nil {
				h.log.Error("Error while unmarshal redis in items aggregation", logger.Error(err))
			} else {
				resp["data"] = m
				h.handleResponse(c, status_http.OK, resp)
				return
			}
		}
	}

	resp, err := service.GetListAggregation(
		context.Background(),
		&obs.CommonMessage{
			TableSlug: c.Param("collection"),
			Data:      structData,
			ProjectId: resource.ResourceEnvironmentId,
		},
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	if reqBody.IsCached {
		jsonData, _ := resp.GetData().MarshalJSON()
		err = h.redis.SetX(context.Background(), base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s-%s-%s", c.Param("collection"), string(key), resource.ResourceEnvironmentId))), string(jsonData), 15*time.Second, projectId.(string), resource.NodeType)
		if err != nil {
			h.log.Error("Error while setting redis in items aggregation", logger.Error(err))
		}
	}

	h.handleResponse(c, status_http.OK, resp)
}

// UpsertManyItems godoc
// @Security ApiKeyAuth
// @ID upsert_many_items
// @Router /v2/items/{collection}/upsert-many [POST]
// @Summary Upsert Many items
// @Description Upsert Many items
// @Tags Items
// @Accept json
// @Produce json
// @Param collection path string true "collection"
// @Param object body models.CommonMessage true "UpsertManyItemsRequestBody"
// @Success 201 {object} status_http.Response{data=models.CommonMessage} "Object data"
// @Response 400 {object} status_http.Response{data=string} "Bad Request"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) UpsertMany(c *gin.Context) {
	var (
		objectRequest models.CommonMessage
		actionErr     error
		functionName  string
	)

	if err := c.ShouldBindJSON(&objectRequest); err != nil {
		h.handleResponse(c, status_http.BadRequest, err.Error())
		return
	}

	projectId, ok := c.Get("project_id")
	if !ok || !util.IsValidUUID(projectId.(string)) {
		h.handleResponse(c, status_http.InvalidArgument, "project id is an invalid uuid")
		return
	}

	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		err := errors.New("error getting environment id | not valid")
		h.handleResponse(c, status_http.BadRequest, err)
		return
	}

	userId, _ := c.Get("user_id")

	resource, err := h.companyServices.ServiceResource().GetSingle(
		c.Request.Context(),
		&pb.GetSingleServiceResourceReq{
			ProjectId:     projectId.(string),
			EnvironmentId: environmentId.(string),
			ServiceType:   pb.ServiceType_BUILDER_SERVICE,
		},
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	services, err := h.GetProjectSrvc(
		c.Request.Context(),
		resource.GetProjectId(),
		resource.NodeType,
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	structData, err := helper.ConvertMapToStruct(objectRequest.Data)
	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	}

	var (
		logReq = &models.CreateVersionHistoryRequest{
			Services:     services,
			NodeType:     resource.NodeType,
			ProjectId:    resource.ResourceEnvironmentId,
			ActionSource: c.Request.URL.String(),
			ActionType:   "UPDATE MANY ITEM",
			UsedEnvironments: map[string]bool{
				cast.ToString(environmentId): true,
			},
			UserInfo:  cast.ToString(userId),
			Request:   &structData,
			TableSlug: c.Param("collection"),
		}
	)

	var resp *obs.CommonMessage

	defer func() {
		if err != nil {
			logReq.Response = err.Error()
			h.handleResponse(c, status_http.GRPCError, err.Error())
		} else if actionErr != nil {
			logReq.Response = actionErr.Error() + " in " + functionName
			h.handleResponse(c, status_http.InvalidArgument, actionErr.Error()+" in "+functionName)
		} else {
			logReq.Response = resp
			h.handleResponse(c, status_http.NoContent, resp)
		}
		go h.versionHistory(logReq)
	}()

	switch resource.ResourceType {
	case pb.ResourceType_POSTGRESQL:
		_, err = services.GoObjectBuilderService().Items().UpsertMany(
			context.Background(),
			&nb.CommonMessage{
				TableSlug: c.Param("collection"),
				Data:      structData,
				ProjectId: resource.ResourceEnvironmentId,
			},
		)

		if err != nil {
			return
		}
	}
}
