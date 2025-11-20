package main

import (
	"context"
	"ucode/ucode_go_client_api_gateway/api"
	"ucode/ucode_go_client_api_gateway/api/handlers"
	"ucode/ucode_go_client_api_gateway/config"
	"ucode/ucode_go_client_api_gateway/genproto/company_service"
	"ucode/ucode_go_client_api_gateway/pkg/caching"
	"ucode/ucode_go_client_api_gateway/pkg/helper"
	"ucode/ucode_go_client_api_gateway/pkg/logger"
	"ucode/ucode_go_client_api_gateway/services"
	"ucode/ucode_go_client_api_gateway/storage/redis"

	"github.com/gin-gonic/gin"
	"github.com/golanguzb70/ratelimiter"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jaeger_config "github.com/uber/jaeger-client-go/config"
)

func main() {
	baseConf := config.BaseLoad()
	conf := config.Load()

	var loggerLevel = new(string)
	*loggerLevel = logger.LevelDebug

	switch baseConf.Environment {
	case config.DebugMode:
		*loggerLevel = logger.LevelDebug
		gin.SetMode(gin.DebugMode)
	case config.TestMode:
		*loggerLevel = logger.LevelDebug
		gin.SetMode(gin.TestMode)
	default:
		*loggerLevel = logger.LevelInfo
		gin.SetMode(gin.ReleaseMode)
	}

	jaegerCfg := &jaeger_config.Configuration{
		ServiceName: baseConf.ServiceName,
		Sampler: &jaeger_config.SamplerConfig{
			Type:  "const",
			Param: 1,
		},
		Reporter: &jaeger_config.ReporterConfig{
			LogSpans:           false,
			LocalAgentHostPort: baseConf.JaegerHostPort,
		},
	}

	log := logger.NewLogger("ucode/ucode_go_client_api_gateway", *loggerLevel)
	defer func() {
		err := logger.Cleanup(log)
		if err != nil {
			return
		}
	}()

	tracer, closer, err := jaegerCfg.NewTracer(jaeger_config.Logger(jaeger.StdLogger))
	if err != nil {
		log.Error("ERROR: cannot init Jaeger", logger.Error(err))
	}
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	uConf := config.Load()

	// auth connection
	authSrvc, err := services.NewAuthServiceClient(ctx, uConf)
	if err != nil {
		log.Error("[ucode] error while establishing auth grpc conn-", logger.Error(err))
		return
	}

	// company connection
	compSrvc, err := services.NewCompanyServiceClient(ctx, uConf)
	if err != nil {
		log.Error("[ucode] error while establishing company grpc conn", logger.Error(err))
		return
	}

	serviceNodes := services.NewServiceNodes()
	grpcSvcs, err := services.NewGrpcClients(ctx, uConf)
	if err != nil {
		log.Error("Error adding grpc client with base config. NewGrpcClients", logger.Error(err))
		return
	}

	err = serviceNodes.Add(grpcSvcs, baseConf.UcodeNamespace)
	if err != nil {
		log.Error("Error adding grpc client to serviceNode. ServiceNode!!", logger.Error(err))
		return
	}

	rps, err := compSrvc.Project().ListProjectsRPS(ctx, &company_service.GetProjectListRequest{})
	if err != nil {
		log.Error("Error getting project list", logger.Error(err))
	}

	// pooling grpc services of enterprice projects
	projectServiceNodes, mapProjectConfs := helper.EnterPriceProjectsGrpcSvcs(ctx, compSrvc, serviceNodes, log)
	if projectServiceNodes == nil {
		projectServiceNodes = serviceNodes
	}

	if mapProjectConfs == nil {
		mapProjectConfs = make(map[string]config.Config)
	}

	mapProjectConfs[baseConf.UcodeNamespace] = uConf

	newRedis := redis.NewRedis(mapProjectConfs, rps.GetProjects())

	cache, err := caching.NewExpiringLRUCache(config.LRU_CACHE_SIZE)
	if err != nil {
		log.Error("Error adding caching.", logger.Error(err))
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	h := handlers.NewHandler(baseConf, mapProjectConfs, log, projectServiceNodes, compSrvc, authSrvc, newRedis, cache)
	cfg := &ratelimiter.Config{
		RedisHost:    conf.GetRequestRedisHost,
		RedisPort:    conf.GetRequestRedisPort,
		JwtSignInKey: "jwt_sign_in_key",
		LeakyBuckets: config.RateLimitCfg,
	}

	limiter, err := ratelimiter.NewRateLimiter(cfg)
	if err != nil {
		log.Panic("Error creating rate limiter", logger.Error(err))
	}

	api.SetUpAPI(r, h, baseConf, tracer, limiter)

	log.Info("server is running...")
	if err := r.Run(baseConf.HTTPPort); err != nil {
		return
	}
}
