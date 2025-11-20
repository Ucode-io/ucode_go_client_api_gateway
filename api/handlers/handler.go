package handlers

import (
	"context"
	v1 "ucode/ucode_go_client_api_gateway/api/handlers/v1"
	v2 "ucode/ucode_go_client_api_gateway/api/handlers/v2"
	"ucode/ucode_go_client_api_gateway/config"
	"ucode/ucode_go_client_api_gateway/pkg/caching"
	"ucode/ucode_go_client_api_gateway/pkg/logger"
	"ucode/ucode_go_client_api_gateway/services"
	"ucode/ucode_go_client_api_gateway/storage"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	baseConf        config.BaseConfig
	projectConfs    map[string]config.Config
	log             logger.LoggerI
	services        services.ServiceNodesI
	companyServices services.CompanyServiceI
	authService     services.AuthServiceI
	redis           storage.RedisStorageI
	V1              v1.HandlerV1
	V2              v2.HandlerV2
	cache           *caching.ExpiringLRUCache
}

func NewHandler(baseConf config.BaseConfig, projectConfs map[string]config.Config, log logger.LoggerI, svcs services.ServiceNodesI, cmpServ services.CompanyServiceI, authService services.AuthServiceI, redis storage.RedisStorageI, cache *caching.ExpiringLRUCache) Handler {
	return Handler{
		baseConf:        baseConf,
		projectConfs:    projectConfs,
		log:             log,
		services:        svcs,
		companyServices: cmpServ,
		authService:     authService,
		redis:           redis,
		V1:              v1.NewHandlerV1(baseConf, projectConfs, log, svcs, cmpServ, authService, redis, cache),
		V2:              v2.NewHandlerV2(baseConf, projectConfs, log, svcs, cmpServ, authService, redis, cache),
		cache:           cache,
	}
}

func (h *Handler) GetCompanyService(c *gin.Context) services.CompanyServiceI {
	return h.companyServices
}

func (h *Handler) GetAuthService(c *gin.Context) services.AuthServiceI {
	return h.authService
}

func (h *Handler) GetProjectConfig(c *gin.Context, projectId string) config.Config {
	return h.projectConfs[projectId]
}

func (h *Handler) GetProjectSrvc(c context.Context, projectId string, nodeType string) (services.ServiceManagerI, error) {
	if nodeType == config.ENTER_PRICE_TYPE {
		srvc, err := h.services.Get(projectId)
		if err != nil {
			return nil, err
		}

		return srvc, nil
	} else {
		srvc, err := h.services.Get(h.baseConf.UcodeNamespace)
		if err != nil {
			return nil, err
		}

		return srvc, nil
	}
}
