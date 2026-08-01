package config

import (
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
)

const defaultHTTPAddr = ":8080"
const defaultStorageDriver = "memory"
const defaultEmbeddingProvider = "local"
const defaultAnswerProvider = "local"
const defaultPlannerProvider = "local"
const defaultRerankProvider = "local"
const defaultEmbeddingDimension = 384
const defaultAuthMode = "dev_headers"
const defaultProjectAuthorizationMode = "disabled"
const defaultJWTUserIDClaim = "sub"
const defaultJWTRolesClaim = "roles"
const defaultJWTClockSkewSeconds = 60
const defaultAgentTaskTimeoutSeconds = 600
const defaultDevelopmentExecutionTimeoutSeconds = 120
const defaultDevelopmentExecutionDriver = "container"
const defaultDevelopmentContainerBinary = "docker"
const defaultDevelopmentContainerCPUs = "1"
const defaultDevelopmentContainerMemory = "1g"
const defaultDevelopmentContainerPIDs = 256
const defaultDevelopmentContainerTmpSize = "256m"
const defaultDevelopmentContainerUser = "65532:65532"
const defaultDevelopmentSecretScannerName = "enterprise-secret-scanner"
const defaultDevelopmentDeliveryRemote = "origin"
const defaultDevelopmentDeliveryTimeoutSeconds = 120
const defaultDevelopmentPRBinary = "gh"

type Config struct {
	HTTPAddr                           string
	StorageDriver                      string
	PostgresDSN                        string
	ImportRoot                         string
	WorkspaceRoot                      string
	EmbeddingProvider                  string
	EmbeddingBaseURL                   string
	EmbeddingAPIKey                    string
	EmbeddingModel                     string
	EmbeddingDimension                 int
	RerankProvider                     string
	RerankBaseURL                      string
	RerankAPIKey                       string
	RerankModel                        string
	AuthMode                           string
	ProjectAuthorizationMode           string
	JWTIssuer                          string
	JWTAudience                        string
	JWTJWKSURL                         string
	JWTUserIDClaim                     string
	JWTRolesClaim                      string
	JWTClockSkewSeconds                int
	JWTAllowInsecureHTTP               bool
	AnswerProvider                     string
	PlannerProvider                    string
	LLMBaseURL                         string
	LLMAPIKey                          string
	LLMModel                           string
	LLMInputUSDPerMillionTokens        float64
	LLMOutputUSDPerMillionTokens       float64
	AgentTaskTimeoutSeconds            int
	DevelopmentExecutionEnabled        bool
	DevelopmentExecutionRoot           string
	DevelopmentExecutionTimeoutSeconds int
	DevelopmentExecutionDriver         string
	DevelopmentContainerBinary         string
	DevelopmentContainerImage          string
	DevelopmentContainerCPUs           string
	DevelopmentContainerMemory         string
	DevelopmentContainerPIDs           int
	DevelopmentContainerTmpSize        string
	DevelopmentContainerUser           string
	DevelopmentContainerGoModCache     string
	DevelopmentSecretScannerName       string
	DevelopmentSecretScannerBinary     string
	DevelopmentSecretScannerArguments  []string
	DevelopmentSecretScannerEnv        []string
	DevelopmentDeliveryEnabled         bool
	DevelopmentDeliveryRoot            string
	DevelopmentDeliveryRemote          string
	DevelopmentDeliveryAuthorName      string
	DevelopmentDeliveryAuthorEmail     string
	DevelopmentDeliveryUsername        string
	DevelopmentDeliveryToken           string
	DevelopmentDeliveryTimeoutSeconds  int
	DevelopmentPRBinary                string
}

func Load() Config {
	address := os.Getenv("EKBDA_HTTP_ADDR")
	if address == "" {
		address = defaultHTTPAddr
	}
	storageDriver := os.Getenv("EKBDA_STORAGE_DRIVER")
	if storageDriver == "" {
		storageDriver = defaultStorageDriver
	}
	embeddingProvider := os.Getenv("EKBDA_EMBEDDING_PROVIDER")
	if embeddingProvider == "" {
		embeddingProvider = defaultEmbeddingProvider
	}
	answerProvider := os.Getenv("EKBDA_ANSWER_PROVIDER")
	if answerProvider == "" {
		answerProvider = defaultAnswerProvider
	}
	rerankProvider := os.Getenv("EKBDA_RERANK_PROVIDER")
	if rerankProvider == "" {
		rerankProvider = defaultRerankProvider
	}
	authMode := os.Getenv("EKBDA_AUTH_MODE")
	if authMode == "" {
		authMode = defaultAuthMode
	}
	userIDClaim := os.Getenv("EKBDA_AUTH_JWT_USER_ID_CLAIM")
	if userIDClaim == "" {
		userIDClaim = defaultJWTUserIDClaim
	}
	rolesClaim := os.Getenv("EKBDA_AUTH_JWT_ROLES_CLAIM")
	if rolesClaim == "" {
		rolesClaim = defaultJWTRolesClaim
	}
	return Config{
		HTTPAddr:                           address,
		StorageDriver:                      storageDriver,
		PostgresDSN:                        os.Getenv("EKBDA_POSTGRES_DSN"),
		ImportRoot:                         os.Getenv("EKBDA_IMPORT_ROOT"),
		WorkspaceRoot:                      os.Getenv("EKBDA_WORKSPACE_ROOT"),
		EmbeddingProvider:                  embeddingProvider,
		EmbeddingBaseURL:                   os.Getenv("EKBDA_EMBEDDING_BASE_URL"),
		EmbeddingAPIKey:                    os.Getenv("EKBDA_EMBEDDING_API_KEY"),
		EmbeddingModel:                     os.Getenv("EKBDA_EMBEDDING_MODEL"),
		EmbeddingDimension:                 positiveInt("EKBDA_EMBEDDING_DIMENSION", defaultEmbeddingDimension),
		RerankProvider:                     rerankProvider,
		RerankBaseURL:                      os.Getenv("EKBDA_RERANK_BASE_URL"),
		RerankAPIKey:                       os.Getenv("EKBDA_RERANK_API_KEY"),
		RerankModel:                        os.Getenv("EKBDA_RERANK_MODEL"),
		AuthMode:                           authMode,
		ProjectAuthorizationMode:           valueOrDefault("EKBDA_PROJECT_AUTHORIZATION_MODE", defaultProjectAuthorizationMode),
		JWTIssuer:                          os.Getenv("EKBDA_AUTH_JWT_ISSUER"),
		JWTAudience:                        os.Getenv("EKBDA_AUTH_JWT_AUDIENCE"),
		JWTJWKSURL:                         os.Getenv("EKBDA_AUTH_JWT_JWKS_URL"),
		JWTUserIDClaim:                     userIDClaim,
		JWTRolesClaim:                      rolesClaim,
		JWTClockSkewSeconds:                positiveInt("EKBDA_AUTH_JWT_CLOCK_SKEW_SECONDS", defaultJWTClockSkewSeconds),
		JWTAllowInsecureHTTP:               os.Getenv("EKBDA_AUTH_JWT_ALLOW_INSECURE_HTTP") == "true",
		AnswerProvider:                     answerProvider,
		PlannerProvider:                    valueOrDefault("EKBDA_PLANNER_PROVIDER", defaultPlannerProvider),
		LLMBaseURL:                         os.Getenv("EKBDA_LLM_BASE_URL"),
		LLMAPIKey:                          os.Getenv("EKBDA_LLM_API_KEY"),
		LLMModel:                           os.Getenv("EKBDA_LLM_MODEL"),
		LLMInputUSDPerMillionTokens:        nonnegativeFloat("EKBDA_LLM_INPUT_USD_PER_MILLION_TOKENS"),
		LLMOutputUSDPerMillionTokens:       nonnegativeFloat("EKBDA_LLM_OUTPUT_USD_PER_MILLION_TOKENS"),
		AgentTaskTimeoutSeconds:            positiveInt("EKBDA_AGENT_TASK_TIMEOUT_SECONDS", defaultAgentTaskTimeoutSeconds),
		DevelopmentExecutionEnabled:        os.Getenv("EKBDA_DEVELOPMENT_EXECUTION_ENABLED") == "true",
		DevelopmentExecutionRoot:           os.Getenv("EKBDA_DEVELOPMENT_EXECUTION_ROOT"),
		DevelopmentExecutionTimeoutSeconds: positiveInt("EKBDA_DEVELOPMENT_EXECUTION_TIMEOUT_SECONDS", defaultDevelopmentExecutionTimeoutSeconds),
		DevelopmentExecutionDriver:         valueOrDefault("EKBDA_DEVELOPMENT_EXECUTION_DRIVER", defaultDevelopmentExecutionDriver),
		DevelopmentContainerBinary:         valueOrDefault("EKBDA_DEVELOPMENT_CONTAINER_BINARY", defaultDevelopmentContainerBinary),
		DevelopmentContainerImage:          os.Getenv("EKBDA_DEVELOPMENT_CONTAINER_IMAGE"),
		DevelopmentContainerCPUs:           valueOrDefault("EKBDA_DEVELOPMENT_CONTAINER_CPUS", defaultDevelopmentContainerCPUs),
		DevelopmentContainerMemory:         valueOrDefault("EKBDA_DEVELOPMENT_CONTAINER_MEMORY", defaultDevelopmentContainerMemory),
		DevelopmentContainerPIDs:           positiveInt("EKBDA_DEVELOPMENT_CONTAINER_PIDS", defaultDevelopmentContainerPIDs),
		DevelopmentContainerTmpSize:        valueOrDefault("EKBDA_DEVELOPMENT_CONTAINER_TMP_SIZE", defaultDevelopmentContainerTmpSize),
		DevelopmentContainerUser:           valueOrDefault("EKBDA_DEVELOPMENT_CONTAINER_USER", defaultDevelopmentContainerUser),
		DevelopmentContainerGoModCache:     os.Getenv("EKBDA_DEVELOPMENT_CONTAINER_GOMODCACHE"),
		DevelopmentSecretScannerName:       valueOrDefault("EKBDA_DEVELOPMENT_SECRET_SCANNER_NAME", defaultDevelopmentSecretScannerName),
		DevelopmentSecretScannerBinary:     os.Getenv("EKBDA_DEVELOPMENT_SECRET_SCANNER_BINARY"),
		DevelopmentSecretScannerArguments:  stringListJSON("EKBDA_DEVELOPMENT_SECRET_SCANNER_ARGS_JSON"),
		DevelopmentSecretScannerEnv:        commaSeparated("EKBDA_DEVELOPMENT_SECRET_SCANNER_ENV_ALLOWLIST"),
		DevelopmentDeliveryEnabled:         os.Getenv("EKBDA_DEVELOPMENT_DELIVERY_ENABLED") == "true",
		DevelopmentDeliveryRoot:            os.Getenv("EKBDA_DEVELOPMENT_DELIVERY_ROOT"),
		DevelopmentDeliveryRemote:          valueOrDefault("EKBDA_DEVELOPMENT_DELIVERY_REMOTE", defaultDevelopmentDeliveryRemote),
		DevelopmentDeliveryAuthorName:      os.Getenv("EKBDA_DEVELOPMENT_DELIVERY_AUTHOR_NAME"),
		DevelopmentDeliveryAuthorEmail:     os.Getenv("EKBDA_DEVELOPMENT_DELIVERY_AUTHOR_EMAIL"),
		DevelopmentDeliveryUsername:        os.Getenv("EKBDA_DEVELOPMENT_DELIVERY_USERNAME"),
		DevelopmentDeliveryToken:           os.Getenv("EKBDA_DEVELOPMENT_DELIVERY_TOKEN"),
		DevelopmentDeliveryTimeoutSeconds:  positiveInt("EKBDA_DEVELOPMENT_DELIVERY_TIMEOUT_SECONDS", defaultDevelopmentDeliveryTimeoutSeconds),
		DevelopmentPRBinary:                valueOrDefault("EKBDA_DEVELOPMENT_PR_BINARY", defaultDevelopmentPRBinary),
	}
}

func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func positiveInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func nonnegativeFloat(name string) float64 {
	value, err := strconv.ParseFloat(os.Getenv(name), 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func stringListJSON(name string) []string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil
	}
	var result []string
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return nil
	}
	return result
}

func commaSeparated(name string) []string {
	values := strings.Split(os.Getenv(name), ",")
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
