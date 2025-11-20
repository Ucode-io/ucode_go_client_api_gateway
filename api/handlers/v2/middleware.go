package v2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"ucode/ucode_go_client_api_gateway/api/status_http"
	"ucode/ucode_go_client_api_gateway/config"
	"ucode/ucode_go_client_api_gateway/genproto/auth_service"
	"ucode/ucode_go_client_api_gateway/genproto/company_service"
	"ucode/ucode_go_client_api_gateway/pkg/helper"
	"ucode/ucode_go_client_api_gateway/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *HandlerV2) LoginMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		resourceId := c.GetHeader("Resource-Id")
		environmentId := c.GetHeader("Environment-Id")
		projectId := c.DefaultQuery("project-id", "")
		clientId := c.GetHeader("Client-id")
		if clientId == "" {
			h.handleResponse(c, status_http.Unauthorized, "The request requires client id")
			c.Abort()
			return
		}

		c.Set("client_id", clientId)
		c.Set("resource_id", resourceId)
		c.Set("environment_id", environmentId)
		c.Set("project_id", projectId)
		c.Next()
	}
}

func (h *HandlerV2) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var (
			res                                  = &auth_service.V2HasAccessUserRes{}
			ok                                   bool
			bearerToken                          = c.GetHeader("Authorization")
			strArr                               = strings.Split(bearerToken, " ")
			resourceId, environmentId, projectId string
		)

		if len(strArr) < 1 && (strArr[0] != "Bearer" && strArr[0] != "API-KEY") {
			h.log.Error("---ERR->Unexpected token format")
			_ = c.AbortWithError(http.StatusForbidden, errors.New("token error: wrong format"))
			return
		}

		switch strArr[0] {
		case "Bearer":
			if res, ok = h.hasAccess(c); !ok {
				c.Abort()
				return
			}

			resourceId = c.GetHeader("Resource-Id")
			environmentId = c.GetHeader("Environment-Id")
			projectId = c.Query("Project-Id")
			userId := c.Query("User-Id")

			if res.ProjectId != "" {
				projectId = res.ProjectId
			}
			if res.EnvId != "" {
				environmentId = res.EnvId
			}
			if res.UserId != "" {
				userId = res.UserIdAuth
			}

			c.Set("user_id", userId)
			c.Set("role_id", res.RoleId)
			c.Set("token", strArr[1])
		case "API-KEY":
			app_id := c.GetHeader("X-API-KEY")

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

			apikeys, err := h.authService.ApiKey().GetEnvID(c.Request.Context(), &auth_service.GetReq{Id: app_id})
			if err != nil {
				h.handleResponse(c, status_http.BadRequest, err.Error())
				c.Abort()
				return
			}

			resource, err := h.companyServices.Resource().GetResourceByEnvID(c.Request.Context(),
				&company_service.GetResourceByEnvIDRequest{
					EnvId: apikeys.GetEnvironmentId(),
				},
			)
			if err != nil {
				h.handleResponse(c, status_http.BadRequest, err.Error())
				c.Abort()
				return
			}

			resourceId = resource.GetResource().GetId()
			environmentId = apikeys.GetEnvironmentId()
			projectId = apikeys.GetProjectId()

			c.Set("client_type_id", apikeys.GetClientTypeId())
			c.Set("role_id", apikeys.GetRoleId())
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

		// redisKey := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s-%s", "monthly", projectId)))
		// redisValue, err := h.redis.Get(c.Request.Context(), redisKey, projectId, "")

		// if err == redis.Nil {
		// 	err = h.redis.Set(c.Request.Context(), redisKey, 1, 0, projectId, "")
		// 	if err != nil {
		// 		h.log.Error("Setting not existing project's request count in redis", logger.Error(err))
		// 	}
		// } else if err != nil {
		// 	h.log.Error("error get project's request count in redis", logger.Error(err))
		// } else {
		// 	count, err := strconv.Atoi(redisValue)
		// 	if err != nil {
		// 		h.log.Error("---ERR->AuthMiddleware->Atoi--->", logger.Error(err))
		// 	} else {
		// 		if count >= config.LIMITER_RANGE {
		// 			err = h.redis.Set(c.Request.Context(), redisKey, 1, 0, projectId, "")
		// 			go func() {
		// 				_, err = h.companyServices.Billing().UpsertMonthlyRequest(c.Request.Context(),
		// 					&company_service.MonthlyRequest{
		// 						ProjectId: projectId,
		// 						Count:     int32(config.LIMITER_RANGE),
		// 					})
		// 				if err != nil {
		// 					h.log.Error("--RateLimiter--UpsertApiKeyUsage", logger.Error(err))
		// 				}
		// 			}()
		// 		} else {
		// 			err = h.redis.Set(c.Request.Context(), redisKey, count+1, 0, projectId, "")
		// 			if err != nil {
		// 				h.log.Error("Setting not existing project's request count in redis", logger.Error(err))
		// 				return
		// 			}
		// 		}
		// 	}
		// }

		// if projectId == "" {
		// 	h.handleResponse(c, status_http.BadRequest, "project id is invalid")
		// 	c.Abort()
		// 	return
		// }

		// allowed, err := h.AllowRequest(c.Request.Context(), projectId)
		// if err != nil {
		// 	fmt.Println("error allow", err)
		// 	h.handleResponse(c, status_http.InternalServerError, err.Error())
		// 	c.Abort()
		// 	return
		// }

		// if !allowed {
		// 	h.handleResponse(c, status_http.TooManyRequests, "Too Many Requests")
		// 	c.Abort()
		// 	return
		// }

		c.Set("resource_id", resourceId)
		c.Set("environment_id", environmentId)
		c.Set("project_id", projectId)
		c.Set("Auth", res)
		c.Set("namespace", h.baseConf.UcodeNamespace)
		c.Next()
	}
}

func (h *HandlerV2) hasAccess(c *gin.Context) (*auth_service.V2HasAccessUserRes, bool) {
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
		h.handleResponse(c, status_http.BadEnvironment, err.Error())
		return nil, false
	}
	defer conn.Close()

	path, tableSlug := helper.GetURLWithTableSlug(c)

	resp, err := service.V2HasAccessUser(
		c.Request.Context(),
		&auth_service.V2HasAccessUserReq{
			AccessToken: accessToken,
			Path:        path,
			Method:      c.Request.Method,
			TableSlug:   tableSlug,
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
		h.log.Error("---ERR->HasAccess->Session->V2HasAccessUser--->", logger.Error(err))
		h.handleResponse(c, status_http.Unauthorized, err.Error())
		return nil, false
	}

	return resp, true
}

func (h *HandlerV2) GetAuthInfo(c *gin.Context) (result *auth_service.V2HasAccessUserRes, err error) {
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

func (h *HandlerV2) AllowRequest(ctx context.Context, projectID string) (bool, error) {
	limitKey := fmt.Sprintf("rate_limit:%s:limit", projectID)
	limit, err := h.redis.GetResult(ctx, limitKey, projectID, "").Int()
	if err == redis.Nil {
		return true, nil
	}

	if err != nil {
		return false, err
	}

	// Increment request count
	countKey := fmt.Sprintf("rate_limit:%s:count", projectID)
	count, err := h.redis.Incr(ctx, countKey, projectID, "").Result()
	if err != nil {
		return false, err
	}

	// If this is the first request in the window, set expiration
	if count == 1 {
		h.redis.Expire(ctx, countKey, config.REDIS_EXPIRATION, projectID, "")
	}

	// Check if request count exceeds the limit
	return count <= int64(limit), nil
}
