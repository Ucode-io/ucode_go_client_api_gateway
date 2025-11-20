package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cast"
)

const (
	// DebugMode indicates service mode is debug.
	DebugMode = "debug"
	// TestMode indicates service mode is test.
	TestMode = "test"
	// ReleaseMode indicates service mode is release.
	ReleaseMode = "release"
)

var CountReq = 0

type Config struct {
	DefaultOffset string
	DefaultLimit  string

	CorporateServiceHost string
	CorporateGRPCPort    string

	ObjectBuilderServiceHost string
	ObjectBuilderGRPCPort    string

	HighObjectBuilderServiceHost string
	HighObjectBuilderGRPCPort    string

	SmsServiceHost string
	SmsGRPCPort    string

	AuthServiceHost string
	AuthGRPCPort    string

	CompanyServiceHost string
	CompanyServicePort string

	FunctionServiceHost string
	FunctionServicePort string

	MinioEndpoint        string
	MinioAccessKeyID     string
	MinioSecretAccessKey string
	MinioProtocol        bool

	UcodeNamespace string

	CLIENT_HOST     string
	SUPERADMIN_HOST string
	SecretKey       string

	PostgresBuilderServiceHost string
	PostgresBuilderServicePort string

	GoObjectBuilderServiceHost string
	GoObjectBuilderGRPCPort    string

	GetRequestRedisHost     string
	GetRequestRedisPort     string
	GetRequestRedisDatabase int
	GetRequestRedisPassword string

	ServiceLink string

	OpenAIApiKey string
}

type BaseConfig struct {
	ServiceName string
	Environment string
	Version     string

	HTTPBaseURL string
	ServiceHost string
	HTTPPort    string
	HTTPScheme  string

	AuthServiceHost string
	AuthGRPCPort    string

	CompanyServiceHost string
	CompanyServicePort string

	MinioEndpoint        string
	MinioAccessKeyID     string
	MinioSecretAccessKey string
	MinioProtocol        bool

	GitlabGroupIdMicroFE   int
	GitlabProjectIdMicroFE int
	GitlabHostMicroFE      string

	DefaultOffset string
	DefaultLimit  string

	GoObjectBuilderServiceHost string
	GoObjectBuilderGRPCPort    string

	CLIENT_HOST     string
	SUPERADMIN_HOST string
	SecretKey       string
	PlatformType    string

	UcodeNamespace string
	JaegerHostPort string

	GithubClientId     string
	GithubClientSecret string
	ProjectUrl         string
	WebhookSecret      string

	ConvertDocxToPdfSecret string

	Email    string
	Password string

	HasuraBaseURL     string
	HasuraAdminSecret string
}

func BaseLoad() BaseConfig {
	if err := godotenv.Load("/app/.env"); err != nil {
		if err := godotenv.Load(".env"); err != nil {
			log.Println("No .env file found")
		}
		log.Println("No /app/.env file found")
	}

	config := BaseConfig{}

	config.ServiceName = cast.ToString(GetOrReturnDefaultValue("SERVICE_NAME", "ucode_go_api_gateway"))
	config.Environment = cast.ToString(GetOrReturnDefaultValue("ENVIRONMENT", DebugMode))
	config.Version = cast.ToString(GetOrReturnDefaultValue("VERSION", "1.0"))

	config.HTTPBaseURL = cast.ToString(GetOrReturnDefaultValue("HTTP_BASE_URL", "https://api.admin.u-code.io"))
	config.ServiceHost = cast.ToString(GetOrReturnDefaultValue("SERVICE_HOST", ""))
	config.HTTPPort = cast.ToString(GetOrReturnDefaultValue("HTTP_PORT", ":8000"))
	config.HTTPScheme = cast.ToString(GetOrReturnDefaultValue("HTTP_SCHEME", ""))

	config.AuthServiceHost = cast.ToString(GetOrReturnDefaultValue("AUTH_SERVICE_HOST", ""))
	config.AuthGRPCPort = cast.ToString(GetOrReturnDefaultValue("AUTH_GRPC_PORT", ""))

	config.CompanyServiceHost = cast.ToString(GetOrReturnDefaultValue("COMPANY_SERVICE_HOST", ""))
	config.CompanyServicePort = cast.ToString(GetOrReturnDefaultValue("COMPANY_GRPC_PORT", ""))

	config.MinioAccessKeyID = cast.ToString(GetOrReturnDefaultValue("MINIO_ACCESS_KEY", ""))
	config.MinioSecretAccessKey = cast.ToString(GetOrReturnDefaultValue("MINIO_SECRET_KEY", ""))
	config.MinioEndpoint = cast.ToString(GetOrReturnDefaultValue("MINIO_ENDPOINT", ""))
	config.MinioProtocol = cast.ToBool(GetOrReturnDefaultValue("MINIO_PROTOCOL", true))

	config.GitlabGroupIdMicroFE = cast.ToInt(GetOrReturnDefaultValue("GITLAB_GROUP_ID_MICROFE", 0))
	config.GitlabProjectIdMicroFE = cast.ToInt(GetOrReturnDefaultValue("GITLAB_PROJECT_ID_MICROFE", 0))
	config.GitlabHostMicroFE = cast.ToString(GetOrReturnDefaultValue("GITLAB_HOST_MICROFE", ""))

	config.GoObjectBuilderServiceHost = cast.ToString(GetOrReturnDefaultValue("GO_OBJECT_BUILDER_SERVICE_GRPC_HOST", "localhost"))
	config.GoObjectBuilderGRPCPort = cast.ToString(GetOrReturnDefaultValue("GO_OBJECT_BUILDER_SERVICE_GRPC_PORT", ":7107"))

	config.DefaultOffset = cast.ToString(GetOrReturnDefaultValue("DEFAULT_OFFSET", "0"))
	config.DefaultLimit = "60"

	config.UcodeNamespace = "u-code"
	config.SecretKey = cast.ToString(GetOrReturnDefaultValue("SECRET_KEY", ""))
	config.JaegerHostPort = cast.ToString(GetOrReturnDefaultValue("JAEGER_URL", ""))

	config.HasuraBaseURL = cast.ToString(GetOrReturnDefaultValue("HASURA_BASE_URL", "http://localhost:8080"))
	config.HasuraAdminSecret = cast.ToString(GetOrReturnDefaultValue("HASURA_ADMIN_SECRET", ""))

	return config
}

// Load ...
func Load() Config {

	config := Config{}

	config.DefaultOffset = cast.ToString(GetOrReturnDefaultValue("DEFAULT_OFFSET", "0"))
	config.DefaultLimit = "100"

	config.CorporateServiceHost = cast.ToString(GetOrReturnDefaultValue("CORPORATE_SERVICE_HOST", ""))
	config.CorporateGRPCPort = cast.ToString(GetOrReturnDefaultValue("CORPORATE_GRPC_PORT", ":2025"))

	config.ObjectBuilderServiceHost = cast.ToString(GetOrReturnDefaultValue("OBJECT_BUILDER_SERVICE_LOW_HOST", ""))
	config.ObjectBuilderGRPCPort = cast.ToString(GetOrReturnDefaultValue("OBJECT_BUILDER_LOW_GRPC_PORT", ""))

	config.AuthServiceHost = cast.ToString(GetOrReturnDefaultValue("AUTH_SERVICE_HOST", ""))
	config.AuthGRPCPort = cast.ToString(GetOrReturnDefaultValue("AUTH_GRPC_PORT", ""))

	config.CompanyServiceHost = cast.ToString(GetOrReturnDefaultValue("COMPANY_SERVICE_HOST", ""))
	config.CompanyServicePort = cast.ToString(GetOrReturnDefaultValue("COMPANY_GRPC_PORT", ""))

	config.HighObjectBuilderServiceHost = cast.ToString(GetOrReturnDefaultValue("OBJECT_BUILDER_SERVICE_HIGHT_HOST", ""))
	config.HighObjectBuilderGRPCPort = cast.ToString(GetOrReturnDefaultValue("OBJECT_BUILDER_HIGH_GRPC_PORT", ""))

	config.SmsServiceHost = cast.ToString(GetOrReturnDefaultValue("SMS_SERVICE_HOST", ""))
	config.SmsGRPCPort = cast.ToString(GetOrReturnDefaultValue("SMS_GRPC_PORT", ":2008"))

	config.FunctionServiceHost = cast.ToString(GetOrReturnDefaultValue("FUNCTION_SERVICE_HOST", ""))
	config.FunctionServicePort = cast.ToString(GetOrReturnDefaultValue("FUNCTION_GRPC_PORT", ":2005"))

	config.GoObjectBuilderServiceHost = cast.ToString(GetOrReturnDefaultValue("GO_OBJECT_BUILDER_SERVICE_GRPC_HOST", "localhost"))
	config.GoObjectBuilderGRPCPort = cast.ToString(GetOrReturnDefaultValue("GO_OBJECT_BUILDER_SERVICE_GRPC_PORT", ":7107"))

	config.UcodeNamespace = "cp-region-type-id"
	config.SecretKey = cast.ToString(GetOrReturnDefaultValue("SECRET_KEY", ""))

	config.GetRequestRedisHost = cast.ToString(GetOrReturnDefaultValue("GET_REQUEST_REDIS_HOST", ""))
	config.GetRequestRedisPort = cast.ToString(GetOrReturnDefaultValue("GET_REQUEST_REDIS_PORT", ""))
	config.GetRequestRedisDatabase = cast.ToInt(GetOrReturnDefaultValue("GET_REQUEST_REDIS_DATABASE", 0))
	config.GetRequestRedisPassword = cast.ToString(GetOrReturnDefaultValue("GET_REQUEST_REDIS_PASSWORD", ""))

	config.ServiceLink = ""

	config.OpenAIApiKey = cast.ToString(GetOrReturnDefaultValue("OPENAI_API_KEY", ""))

	return config
}

func GetOrReturnDefaultValue(key string, defaultValue interface{}) interface{} {
	val, exists := os.LookupEnv(key)

	if exists {
		return val
	}

	return defaultValue
}
