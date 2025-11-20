package api

import (
	"encoding/json"
	"errors"
	"strings"

	"ucode/ucode_go_client_api_gateway/api/docs"
	"ucode/ucode_go_client_api_gateway/api/handlers"
	"ucode/ucode_go_client_api_gateway/config"
	"ucode/ucode_go_client_api_gateway/pkg/helper"

	"github.com/gin-gonic/gin"
	"github.com/golanguzb70/ratelimiter"
	"github.com/opentracing-contrib/go-gin/ginhttp"
	"github.com/opentracing/opentracing-go"
	"github.com/spf13/cast"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/gin-swagger/swaggerFiles"
)

// SetUpAPI @description This is an api gateway
// @termsOfService https://udevs.io
func SetUpAPI(r *gin.Engine, h handlers.Handler, cfg config.BaseConfig, tracer opentracing.Tracer, limiter ratelimiter.RateLimiterI) {
	docs.SwaggerInfo.Title = cfg.ServiceName
	docs.SwaggerInfo.Version = cfg.Version
	docs.SwaggerInfo.Schemes = []string{cfg.HTTPScheme}

	r.Use(customCORSMiddleware())
	r.Use(ginhttp.Middleware(tracer))

	r.Any("/x-api/*any", h.V1.RedirectAuthMiddleware(cfg), proxyMiddleware(r, &h), h.V1.Proxy)

	// @securityDefinitions.apikey ApiKeyAuth
	// @in header
	// @name Authorization

	// graphql := r.Group("/v1/graphql")
	// graphql.Use(h.V1.AuthMiddleware(cfg))
	// {
	// 	graphql.POST("", h.V1.Graphql)
	// }

	r.POST("/handler/:path", h.V2.HasuraEvent)

	v1 := r.Group("/v1")
	v1.Use(h.V1.AuthMiddleware(cfg))
	{
		// INVOKE FUNCTION
		v1.POST("/invoke_function/:function-path", h.V1.InvokeFunctionByPath)

		v1.POST("/files/folder_upload", h.V1.UploadToFolder)
		v1.GET("/files/:id", h.V1.GetSingleFile)
		v1.PUT("/files", h.V1.UpdateFile)
		v1.DELETE("/files", h.V1.DeleteFiles)
		v1.DELETE("/files/:id", h.V1.DeleteFile)
		v1.GET("/files", h.V1.GetAllFiles)
	}

	v2 := r.Group("/v2")
	v2.Use(h.V2.AuthMiddleware())

	// items group
	v2Items := v2.Group("/items")
	{
		v2Items.GET("/:collection", h.V2.GetAllItems)
		v2Items.GET("/:collection/:id", h.V2.GetSingleItem)
		v2Items.POST("/:collection", h.V2.CreateItem)
		v2Items.POST("/:collection/multiple-insert", h.V2.CreateItems)
		v2Items.POST("/:collection/upsert-many", h.V2.UpsertMany)
		v2Items.PUT("/:collection", h.V2.UpdateItem)
		v2Items.PUT("/:collection/:id", h.V2.UpdateItem)
		v2Items.PATCH("/:collection", h.V2.MultipleUpdateItems)
		v2Items.PATCH("/:collection/:id", h.V2.UpdateItem)
		v2Items.DELETE("/:collection", h.V2.DeleteItems)
		v2Items.DELETE("/:collection/:id", h.V2.DeleteItem)
		v2Items.POST("/:collection/aggregation", h.V2.GetListAggregation)
		v2Items.POST("/:collection/graphql", h.V2.Graphql)
	}

	{
		v2.POST("/refresh", h.V2.RefreshToken)
		v2.POST("/logout", h.V2.Logout)
		v2.PUT("/refresh", h.V2.RefreshToken)
		v2.POST("/forgot-password", h.V2.ForgotPassword)
	}

	login := r.Group("/v2")
	login.Use(h.V2.LoginMiddleware())
	{
		login.POST("/register", h.V2.Register)
		login.POST("/login", h.V2.Login)
		login.POST("/login/with-option", h.V2.LoginWithOption)
	}

	v2Sms := r.Group("/v2")
	v2Sms.Use(h.V2.LoginMiddleware(), limiter.GinMiddleware())
	{
		v2Sms.POST("/send-code", h.V2.SendCode)
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

func customCORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "3600")
		c.Header("Access-Control-Allow-Methods", "*")
		c.Header("Access-Control-Allow-Headers", "*")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func proxyMiddleware(r *gin.Engine, h *handlers.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		var (
			err error
		)
		c, err = RedirectUrl(c, h)
		if err == nil {
			r.HandleContext(c)
		}
		c.Next()
	}
}

func RedirectUrl(c *gin.Context, h *handlers.Handler) (*gin.Context, error) {
	path := c.Request.URL.Path
	projectId, ok := c.Get("project_id")
	if !ok {
		return c, errors.New("something went wrong")
	}

	envId, ok := c.Get("environment_id")
	if !ok {
		return c, errors.New("something went wrong")
	}

	c.Request.Header.Add("prev_path", path)
	data := helper.MatchingData{
		ProjectId: projectId.(string),
		EnvId:     envId.(string),
		Path:      path,
	}

	res, err := h.V1.CompanyRedirectGetList(data, h.GetCompanyService(c))
	if err != nil {
		return c, errors.New("cant change")
	}

	pathM, err := helper.FindUrlTo(res, data)
	if err != nil {
		return c, errors.New("cant change")
	}
	if path == pathM {
		return c, errors.New("identical path")
	}

	c.Request.URL.Path = pathM
	if strings.Contains(pathM, "/v1/functions/") {
		c.Request.Header.Add("/v1/functions/", cast.ToString(true))
	}

	c.Request.Header.Add("resource_id", cast.ToString(c.Value("resource_id")))
	c.Request.Header.Add("environment_id", cast.ToString(c.Value("environment_id")))
	c.Request.Header.Add("project_id", cast.ToString(c.Value("project_id")))
	c.Request.Header.Add("resource", cast.ToString(c.Value("resource")))
	c.Request.Header.Add("redirect", cast.ToString(true))

	auth, err := json.Marshal(c.Value("auth"))
	if err != nil {
		return c, errors.New("something went wrong")
	}
	c.Request.Header.Add("auth", string(auth))
	return c, nil
}
