package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"ucode/ucode_go_client_api_gateway/api/models"
	"ucode/ucode_go_client_api_gateway/api/status_http"
	"ucode/ucode_go_client_api_gateway/config"
	"ucode/ucode_go_client_api_gateway/genproto/auth_service"
	"ucode/ucode_go_client_api_gateway/genproto/company_service"
	"ucode/ucode_go_client_api_gateway/pkg/helper"
	"ucode/ucode_go_client_api_gateway/pkg/logger"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *HandlerV1) AuthMiddleware(cfg config.BaseConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		var (
			res = &auth_service.V2HasAccessUserRes{}
			ok  bool
		)

		bearerToken := c.GetHeader("Authorization")
		app_id := c.GetHeader("X-API-KEY")

		strArr := strings.Split(bearerToken, " ")

		if len(strArr) < 1 && (strArr[0] != "Bearer" && strArr[0] != "API-KEY") {
			h.log.Error("---ERR->Unexpected token format")
			_ = c.AbortWithError(http.StatusForbidden, errors.New("token error: wrong format"))
			return
		}

		switch strArr[0] {
		case "Bearer":
			res, ok = h.hasAccess(c)
			if !ok {
				h.log.Error("---ERR->AuthMiddleware->hasNotAccess-->")
				c.Abort()
				return
			}

			resourceId := c.GetHeader("Resource-Id")
			environmentId := c.GetHeader("Environment-Id")
			projectId := c.Query("project-id")

			if res.ProjectId != "" {
				projectId = res.ProjectId
			}
			if res.EnvId != "" {
				environmentId = res.EnvId
			}

			c.Set("resource_id", resourceId)
			c.Set("environment_id", environmentId)
			c.Set("project_id", projectId)
			c.Set("user_id", res.UserIdAuth)
		case "API-KEY":
			if app_id == "" {
				err := errors.New("error invalid api-key method")
				h.log.Error("--AuthMiddleware--", logger.Error(err))
				c.JSON(401, struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				}{
					Code:    401,
					Message: "The request requires an user authentication.",
				})
				c.Abort()
				return
			}

			var (
				appIdKey, resourceAppIdKey = app_id, app_id + "resource"

				err      error
				apiJson  []byte
				apikeys  = &auth_service.GetRes{}
				resource = &company_service.GetResourceByEnvIDResponse{}
			)

			var appWaitkey = config.CACHE_WAIT + "-appID"
			_, appIdOk := h.cache.Get(appWaitkey)
			if !appIdOk {
				h.cache.Add(appWaitkey, []byte(appWaitkey), config.REDIS_KEY_TIMEOUT)
			}

			if appIdOk {
				ctx, cancel := context.WithTimeout(context.Background(), config.REDIS_WAIT_TIMEOUT)
				defer cancel()

				for {
					appIdBody, ok := h.cache.Get(appIdKey)
					if ok {
						apiJson = appIdBody
						err = json.Unmarshal(appIdBody, &apikeys)
						if err != nil {
							h.handleResponse(c, status_http.BadRequest, "cant get auth info")
							c.Abort()
							return
						}
					}

					if apikeys.AppId != "" {
						break
					}

					if ctx.Err() == context.DeadlineExceeded {
						break
					}

					time.Sleep(config.REDIS_SLEEP)
				}
			}

			if apikeys.AppId == "" {
				apikeys, err = h.authService.ApiKey().GetEnvID(
					c.Request.Context(),
					&auth_service.GetReq{
						Id: app_id,
					},
				)
				if err != nil {
					h.handleResponse(c, status_http.BadRequest, err.Error())
					c.Abort()
					return
				}

				apiJson, err = json.Marshal(apikeys)
				if err != nil {
					h.handleResponse(c, status_http.BadRequest, "cant get auth info")
					c.Abort()
					return
				}

				go func() {
					h.cache.Add(appIdKey, apiJson, config.REDIS_TIMEOUT)
				}()
			}

			var resourceWaitKey = config.CACHE_WAIT + "-resource"
			_, resourceOk := h.cache.Get(resourceWaitKey)
			if !resourceOk {
				h.cache.Add(resourceWaitKey, []byte(resourceWaitKey), config.REDIS_KEY_TIMEOUT)
			}

			if resourceOk {
				ctx, cancel := context.WithTimeout(context.Background(), config.REDIS_WAIT_TIMEOUT)
				defer cancel()

				for {
					resourceBody, ok := h.cache.Get(resourceAppIdKey)
					if ok {
						err = json.Unmarshal(resourceBody, &resource)
						if err != nil {
							h.handleResponse(c, status_http.BadRequest, "cant get auth info")
							c.Abort()
							return
						}
					}

					if resource.Resource != nil {
						break
					}

					if ctx.Err() == context.DeadlineExceeded {
						break
					}

					time.Sleep(config.REDIS_SLEEP)
				}
			}

			if resource.Resource == nil {
				resource, err = h.companyServices.Resource().GetResourceByEnvID(
					c.Request.Context(),
					&company_service.GetResourceByEnvIDRequest{
						EnvId: apikeys.GetEnvironmentId(),
					},
				)
				if err != nil {
					h.handleResponse(c, status_http.BadRequest, err.Error())
					c.Abort()
					return
				}

				go func() {
					resourceBody, err := json.Marshal(resource)
					if err != nil {
						h.handleResponse(c, status_http.BadRequest, "cant get auth info")
						return
					}
					h.cache.Add(resourceAppIdKey, resourceBody, config.REDIS_TIMEOUT)
				}()
			}

			data := make(map[string]interface{})
			err = json.Unmarshal(apiJson, &data)
			if err != nil {
				h.handleResponse(c, status_http.BadRequest, "cant get auth info")
				c.Abort()
				return
			}

			resourceBody, err := json.Marshal(resource)
			if err != nil {
				h.handleResponse(c, status_http.BadRequest, "cant get auth info")
				return
			}

			c.Set("auth", models.AuthData{Type: "API-KEY", Data: data})
			c.Set("resource_id", resource.GetResource().GetId())
			c.Set("environment_id", apikeys.GetEnvironmentId())
			c.Set("project_id", apikeys.GetProjectId())
			c.Set("resource", string(resourceBody))
		default:
			if !strings.Contains(c.Request.URL.Path, "api") {
				err := errors.New("error invalid authorization method")
				h.log.Error("--AuthMiddleware--", logger.Error(err))
				h.handleResponse(c, status_http.BadRequest, err.Error())
				c.Abort()
			} else {
				err := errors.New("error invalid authorization method")
				h.log.Error("--AuthMiddleware--", logger.Error(err))
				c.JSON(401, struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				}{
					Code:    401,
					Message: "The request requires an user authentication.",
				})
				c.Abort()
			}

		}
		c.Set("Auth", res)

		c.Next()
	}
}

func (h *HandlerV1) RedirectAuthMiddleware(cfg config.BaseConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		var (
			res = &auth_service.V2HasAccessUserRes{}
			//platformType = c.GetHeader("Platform-Type")
		)
		app_id := c.DefaultQuery("x-api-key", "")
		if app_id == "" {
			err := errors.New("error invalid api-key method")
			h.log.Error("--AuthMiddleware--", logger.Error(err))
			c.JSON(401, struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}{
				Code:    401,
				Message: "The request requires an user authentication.",
			})
			c.Abort()
			return
		}
		var (
			appIdKey, resourceAppIdKey = app_id, app_id + "resource"

			err      error
			apiJson  []byte
			apikeys  = &auth_service.GetRes{}
			resource = &company_service.GetResourceByEnvIDResponse{}
		)

		var appWaitkey = config.CACHE_WAIT + "-appID"
		_, appIdOk := h.cache.Get(appWaitkey)
		if !appIdOk {
			h.cache.Add(appWaitkey, []byte(appWaitkey), config.REDIS_KEY_TIMEOUT)
		}
		if appIdOk {
			ctx, cancel := context.WithTimeout(context.Background(), config.REDIS_WAIT_TIMEOUT)
			defer cancel()

			for {
				appIdBody, ok := h.cache.Get(appIdKey)
				if ok {
					apiJson = appIdBody
					err = json.Unmarshal(appIdBody, &apikeys)
					if err != nil {
						h.handleResponse(c, status_http.BadRequest, "cant get auth info")
						c.Abort()
						return
					}
				}

				if apikeys.AppId != "" {
					break
				}

				if ctx.Err() == context.DeadlineExceeded {
					break
				}

				time.Sleep(config.REDIS_SLEEP)
			}
		}
		if apikeys.AppId == "" {
			apikeys, err = h.authService.ApiKey().GetEnvID(
				c.Request.Context(),
				&auth_service.GetReq{
					Id: app_id,
				},
			)
			if err != nil {
				h.handleResponse(c, status_http.BadRequest, err.Error())
				c.Abort()
				return
			}

			apiJson, err = json.Marshal(apikeys)
			if err != nil {
				h.handleResponse(c, status_http.BadRequest, "cant get auth info")
				c.Abort()
				return
			}

			go func() {
				h.cache.Add(appIdKey, apiJson, config.REDIS_TIMEOUT)
			}()
		}

		var resourceWaitKey = config.CACHE_WAIT + "-resource"
		_, resourceOk := h.cache.Get(resourceWaitKey)
		if !resourceOk {
			h.cache.Add(resourceWaitKey, []byte(resourceWaitKey), config.REDIS_KEY_TIMEOUT)
		}

		if resourceOk {
			ctx, cancel := context.WithTimeout(context.Background(), config.REDIS_WAIT_TIMEOUT)
			defer cancel()

			for {
				resourceBody, ok := h.cache.Get(resourceAppIdKey)
				if ok {
					err = json.Unmarshal(resourceBody, &resource)
					if err != nil {
						h.handleResponse(c, status_http.BadRequest, "cant get auth info")
						c.Abort()
						return
					}
				}

				if resource.Resource != nil {
					break
				}

				if ctx.Err() == context.DeadlineExceeded {
					break
				}

				time.Sleep(config.REDIS_SLEEP)
			}
		}
		if resource.Resource == nil {
			resource, err := h.companyServices.Resource().GetResourceByEnvID(
				c.Request.Context(),
				&company_service.GetResourceByEnvIDRequest{
					EnvId: apikeys.GetEnvironmentId(),
				},
			)
			if err != nil {
				h.handleResponse(c, status_http.BadRequest, err.Error())
				c.Abort()
				return
			}

			go func() {
				resourceBody, err := json.Marshal(resource)
				if err != nil {
					h.handleResponse(c, status_http.BadRequest, "cant get auth info")
					return
				}
				h.cache.Add(resourceAppIdKey, resourceBody, config.REDIS_TIMEOUT)
			}()
		}
		data := make(map[string]interface{})
		err = json.Unmarshal(apiJson, &data)
		if err != nil {
			h.handleResponse(c, status_http.BadRequest, "cant get auth info")
			c.Abort()
			return
		}

		c.Set("auth", models.AuthData{Type: "API-KEY", Data: data})
		c.Set("resource_id", resource.GetResource().GetId())
		c.Set("environment_id", apikeys.GetEnvironmentId())
		c.Set("project_id", apikeys.GetProjectId())

		c.Set("Auth", res)
		// c.Set("namespace", h.cfg.UcodeNamespace)

		c.Next()
	}
}

func (h *HandlerV1) hasAccess(c *gin.Context) (*auth_service.V2HasAccessUserRes, bool) {
	bearerToken := c.GetHeader("Authorization")
	strArr := strings.Split(bearerToken, " ")

	if len(strArr) != 2 || strArr[0] != "Bearer" {
		h.log.Error("---ERR->HasAccess->Unexpected token format")
		h.handleResponse(c, status_http.Forbidden, "token error: wrong format")
		return nil, false
	}

	accessToken := strArr[1]
	service, conn, err := h.authService.Session(c)
	if err != nil {
		return nil, false
	}
	defer conn.Close()

	path, tableSlug := helper.GetURLWithTableSlug(c)

	resp, err := service.V2HasAccessUser(
		c.Request.Context(),
		&auth_service.V2HasAccessUserReq{
			AccessToken:   accessToken,
			Path:          path,
			Method:        c.Request.Method,
			ProjectId:     c.Query("project-id"),
			EnvironmentId: c.GetHeader("Environment-Id"),
			TableSlug:     tableSlug,
		},
	)
	if err != nil {
		errr := status.Error(codes.PermissionDenied, "Permission denied")
		if errr.Error() == err.Error() {
			h.log.Error("---ERR->HasAccess->Permission--->", logger.Error(err))
			h.handleResponse(c, status_http.BadRequest, err.Error())
			return nil, false
		}
		errr = status.Error(codes.InvalidArgument, "User has been expired")
		if errr.Error() == err.Error() {
			h.log.Error("---ERR->HasAccess->User Expired-->")
			h.handleResponse(c, status_http.Forbidden, err.Error())
			return nil, false
		}
		errr = status.Error(codes.Unavailable, "User not access environment")
		if errr.Error() == err.Error() {
			h.log.Error("---ERR->HasAccess->User not access environment-->")
			h.handleResponse(c, status_http.Unauthorized, err.Error())
			return nil, false
		}
		h.log.Error("---ERR->HasAccess->Session->V2HasAccessUser--->", logger.Error(err))
		h.handleResponse(c, status_http.Unauthorized, err.Error())
		return nil, false
	}

	return resp, true
}

func (h *HandlerV1) GetAuthInfo(c *gin.Context) (result *auth_service.V2HasAccessUserRes, err error) {
	data, ok := c.Get("Auth")
	if !ok {
		h.handleResponse(c, status_http.Forbidden, "token error: wrong format")
		c.Abort()
		return nil, errors.New("token error: wrong format")
	}

	accessResponse, ok := data.(*auth_service.V2HasAccessUserRes)
	if !ok {
		h.handleResponse(c, status_http.Forbidden, "token error: wrong format")
		c.Abort()
		return nil, errors.New("token error: wrong format")
	}

	return accessResponse, nil
}
