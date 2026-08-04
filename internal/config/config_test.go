package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("EKBDA_HTTP_ADDR", "")
	t.Setenv("EKBDA_STORAGE_DRIVER", "")
	t.Setenv("EKBDA_POSTGRES_DSN", "")
	t.Setenv("EKBDA_IMPORT_ROOT", "")
	t.Setenv("EKBDA_WORKSPACE_ROOT", "")
	t.Setenv("EKBDA_EMBEDDING_PROVIDER", "")
	t.Setenv("EKBDA_EMBEDDING_DIMENSION", "")
	t.Setenv("EKBDA_RERANK_PROVIDER", "")
	t.Setenv("EKBDA_AUTH_MODE", "")
	t.Setenv("EKBDA_PROJECT_AUTHORIZATION_MODE", "")
	t.Setenv("EKBDA_AUTH_JWT_USER_ID_CLAIM", "")
	t.Setenv("EKBDA_AUTH_JWT_ROLES_CLAIM", "")
	t.Setenv("EKBDA_AUTH_JWT_CLOCK_SKEW_SECONDS", "")
	t.Setenv("EKBDA_ANSWER_PROVIDER", "")
	t.Setenv("EKBDA_PLANNER_PROVIDER", "")
	t.Setenv("EKBDA_AGENT_TASK_TIMEOUT_SECONDS", "")
	t.Setenv("EKBDA_DEVELOPMENT_EXECUTION_ENABLED", "")
	t.Setenv("EKBDA_DEVELOPMENT_EXECUTION_ROOT", "")
	t.Setenv("EKBDA_DEVELOPMENT_EXECUTION_TIMEOUT_SECONDS", "")
	t.Setenv("EKBDA_DEVELOPMENT_EXECUTION_DRIVER", "")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_BINARY", "")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_IMAGE", "")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_CPUS", "")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_MEMORY", "")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_PIDS", "")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_TMP_SIZE", "")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_USER", "")
	t.Setenv("EKBDA_DEVELOPMENT_SECRET_SCANNER_ARGS_JSON", "")
	t.Setenv("EKBDA_DEVELOPMENT_SECRET_SCANNER_ENV_ALLOWLIST", "")
	t.Setenv("EKBDA_DEVELOPMENT_DELIVERY_ENABLED", "")
	t.Setenv("EKBDA_DEVELOPMENT_DELIVERY_REMOTE", "")
	t.Setenv("EKBDA_DEVELOPMENT_DELIVERY_TIMEOUT_SECONDS", "")
	t.Setenv("EKBDA_DEVELOPMENT_PR_BINARY", "")
	t.Setenv("EKBDA_RELEASE_ENABLED", "")
	t.Setenv("EKBDA_RELEASE_PIPELINES", "")
	t.Setenv("EKBDA_RELEASE_ENVIRONMENTS", "")
	t.Setenv("EKBDA_RELEASE_TIMEOUT_SECONDS", "")
	t.Setenv("EKBDA_RELEASE_WEBHOOK_MAX_AGE_SECONDS", "")

	config := Load()
	if config.HTTPAddr != ":8080" {
		t.Fatalf("unexpected HTTP address: %q", config.HTTPAddr)
	}
	if config.StorageDriver != "memory" {
		t.Fatalf("unexpected storage driver: %q", config.StorageDriver)
	}
	if config.EmbeddingProvider != "local" {
		t.Fatalf("unexpected embedding provider: %q", config.EmbeddingProvider)
	}
	if config.AnswerProvider != "local" {
		t.Fatalf("unexpected answer provider: %q", config.AnswerProvider)
	}
	if config.PlannerProvider != "local" {
		t.Fatalf("unexpected planner provider: %q", config.PlannerProvider)
	}
	if config.EmbeddingDimension != 384 || config.RerankProvider != "local" {
		t.Fatalf("unexpected retrieval defaults: %#v", config)
	}
	if config.AuthMode != "dev_headers" || config.JWTUserIDClaim != "sub" || config.JWTRolesClaim != "roles" || config.JWTClockSkewSeconds != 60 {
		t.Fatalf("unexpected authentication defaults: %#v", config)
	}
	if config.ProjectAuthorizationMode != "disabled" {
		t.Fatalf("unexpected project authorization mode: %q", config.ProjectAuthorizationMode)
	}
	if config.AgentTaskTimeoutSeconds != 600 {
		t.Fatalf("unexpected agent task timeout: %d", config.AgentTaskTimeoutSeconds)
	}
	if config.DevelopmentExecutionEnabled || config.DevelopmentExecutionRoot != "" || config.DevelopmentExecutionTimeoutSeconds != 120 {
		t.Fatalf("unexpected development execution defaults: %#v", config)
	}
	if config.DevelopmentExecutionDriver != "container" || config.DevelopmentContainerBinary != "docker" || config.DevelopmentContainerCPUs != "1" || config.DevelopmentContainerMemory != "1g" || config.DevelopmentContainerPIDs != 256 || config.DevelopmentContainerTmpSize != "256m" || config.DevelopmentContainerUser != "65532:65532" || config.DevelopmentDeliveryEnabled || config.DevelopmentDeliveryRemote != "origin" || config.DevelopmentDeliveryTimeoutSeconds != 120 || config.DevelopmentPRBinary != "gh" {
		t.Fatalf("unexpected stage 8C defaults: %#v", config)
	}
	if config.ReleaseEnabled || config.ReleaseTimeoutSeconds != 120 || config.ReleaseWebhookMaxAgeSeconds != 300 || len(config.ReleasePipelines) != 0 || len(config.ReleaseEnvironments) != 0 {
		t.Fatalf("unexpected stage 8D defaults: %#v", config)
	}
}

func TestLoadPostgres(t *testing.T) {
	t.Setenv("EKBDA_HTTP_ADDR", ":9090")
	t.Setenv("EKBDA_STORAGE_DRIVER", "postgres")
	t.Setenv("EKBDA_POSTGRES_DSN", "postgres://example")
	t.Setenv("EKBDA_IMPORT_ROOT", "C:/knowledge")
	t.Setenv("EKBDA_WORKSPACE_ROOT", "C:/workspaces")
	t.Setenv("EKBDA_EMBEDDING_PROVIDER", "openai-compatible")
	t.Setenv("EKBDA_EMBEDDING_BASE_URL", "https://embedding.example/v1")
	t.Setenv("EKBDA_EMBEDDING_API_KEY", "secret")
	t.Setenv("EKBDA_EMBEDDING_MODEL", "embedding-model")
	t.Setenv("EKBDA_EMBEDDING_DIMENSION", "768")
	t.Setenv("EKBDA_RERANK_PROVIDER", "http")
	t.Setenv("EKBDA_RERANK_BASE_URL", "https://rerank.example/v1")
	t.Setenv("EKBDA_RERANK_API_KEY", "rerank-secret")
	t.Setenv("EKBDA_RERANK_MODEL", "rerank-model")
	t.Setenv("EKBDA_AUTH_MODE", "jwt")
	t.Setenv("EKBDA_PROJECT_AUTHORIZATION_MODE", "enforced")
	t.Setenv("EKBDA_AUTH_JWT_ISSUER", "https://sso.example")
	t.Setenv("EKBDA_AUTH_JWT_AUDIENCE", "ekbda-api")
	t.Setenv("EKBDA_AUTH_JWT_JWKS_URL", "https://sso.example/jwks")
	t.Setenv("EKBDA_AUTH_JWT_USER_ID_CLAIM", "employee_id")
	t.Setenv("EKBDA_AUTH_JWT_ROLES_CLAIM", "realm_access.roles")
	t.Setenv("EKBDA_AUTH_JWT_CLOCK_SKEW_SECONDS", "30")
	t.Setenv("EKBDA_AUTH_JWT_ALLOW_INSECURE_HTTP", "true")
	t.Setenv("EKBDA_ANSWER_PROVIDER", "openai-compatible")
	t.Setenv("EKBDA_PLANNER_PROVIDER", "openai-compatible")
	t.Setenv("EKBDA_LLM_BASE_URL", "https://llm.example/v1")
	t.Setenv("EKBDA_LLM_API_KEY", "secret")
	t.Setenv("EKBDA_LLM_MODEL", "chat-model")
	t.Setenv("EKBDA_LLM_INPUT_USD_PER_MILLION_TOKENS", "2.5")
	t.Setenv("EKBDA_LLM_OUTPUT_USD_PER_MILLION_TOKENS", "10")
	t.Setenv("EKBDA_AGENT_TASK_TIMEOUT_SECONDS", "900")
	t.Setenv("EKBDA_DEVELOPMENT_EXECUTION_ENABLED", "true")
	t.Setenv("EKBDA_DEVELOPMENT_EXECUTION_ROOT", "C:/executions")
	t.Setenv("EKBDA_DEVELOPMENT_EXECUTION_TIMEOUT_SECONDS", "180")
	t.Setenv("EKBDA_DEVELOPMENT_EXECUTION_DRIVER", "container")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_BINARY", "podman")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_IMAGE", "registry.example/ekbda-go@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_CPUS", "2")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_MEMORY", "2g")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_PIDS", "128")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_TMP_SIZE", "512m")
	t.Setenv("EKBDA_DEVELOPMENT_CONTAINER_USER", "10001:10001")
	t.Setenv("EKBDA_DEVELOPMENT_SECRET_SCANNER_ARGS_JSON", `["scan","{repository}"]`)
	t.Setenv("EKBDA_DEVELOPMENT_SECRET_SCANNER_ENV_ALLOWLIST", "SCANNER_TOKEN, SCANNER_LICENSE")
	t.Setenv("EKBDA_DEVELOPMENT_DELIVERY_ENABLED", "true")
	t.Setenv("EKBDA_DEVELOPMENT_DELIVERY_ROOT", "C:/deliveries")
	t.Setenv("EKBDA_DEVELOPMENT_DELIVERY_REMOTE", "upstream")
	t.Setenv("EKBDA_DEVELOPMENT_DELIVERY_TIMEOUT_SECONDS", "240")
	t.Setenv("EKBDA_DEVELOPMENT_PR_BINARY", "gh-enterprise")
	t.Setenv("EKBDA_RELEASE_ENABLED", "true")
	t.Setenv("EKBDA_RELEASE_PROVIDER_BASE_URL", "https://cicd.example")
	t.Setenv("EKBDA_RELEASE_PROVIDER_TOKEN", "provider-token")
	t.Setenv("EKBDA_RELEASE_WEBHOOK_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("EKBDA_RELEASE_CODE_WEBHOOK_SECRET", "abcdef0123456789abcdef0123456789")
	t.Setenv("EKBDA_RELEASE_PIPELINES", "build-deploy, rollback")
	t.Setenv("EKBDA_RELEASE_ENVIRONMENTS", "staging, production")
	t.Setenv("EKBDA_RELEASE_TIMEOUT_SECONDS", "180")
	t.Setenv("EKBDA_RELEASE_WEBHOOK_MAX_AGE_SECONDS", "240")

	config := Load()
	if config.HTTPAddr != ":9090" || config.StorageDriver != "postgres" || config.PostgresDSN != "postgres://example" || config.ImportRoot != "C:/knowledge" || config.WorkspaceRoot != "C:/workspaces" || config.EmbeddingProvider != "openai-compatible" || config.EmbeddingModel != "embedding-model" || config.EmbeddingDimension != 768 || config.RerankProvider != "http" || config.RerankModel != "rerank-model" || config.AuthMode != "jwt" || config.ProjectAuthorizationMode != "enforced" || config.JWTIssuer != "https://sso.example" || config.JWTAudience != "ekbda-api" || config.JWTJWKSURL != "https://sso.example/jwks" || config.JWTUserIDClaim != "employee_id" || config.JWTRolesClaim != "realm_access.roles" || config.JWTClockSkewSeconds != 30 || !config.JWTAllowInsecureHTTP || config.AnswerProvider != "openai-compatible" || config.PlannerProvider != "openai-compatible" || config.LLMModel != "chat-model" || config.LLMInputUSDPerMillionTokens != 2.5 || config.LLMOutputUSDPerMillionTokens != 10 || config.AgentTaskTimeoutSeconds != 900 || !config.DevelopmentExecutionEnabled || config.DevelopmentExecutionRoot != "C:/executions" || config.DevelopmentExecutionTimeoutSeconds != 180 {
		t.Fatalf("unexpected config: %#v", config)
	}
	if config.DevelopmentContainerBinary != "podman" || config.DevelopmentContainerCPUs != "2" || config.DevelopmentContainerPIDs != 128 || len(config.DevelopmentSecretScannerArguments) != 2 || len(config.DevelopmentSecretScannerEnv) != 2 || !config.DevelopmentDeliveryEnabled || config.DevelopmentDeliveryRoot != "C:/deliveries" || config.DevelopmentDeliveryRemote != "upstream" || config.DevelopmentDeliveryTimeoutSeconds != 240 || config.DevelopmentPRBinary != "gh-enterprise" {
		t.Fatalf("unexpected stage 8C config: %#v", config)
	}
	if !config.ReleaseEnabled || config.ReleaseProviderBaseURL != "https://cicd.example" || len(config.ReleasePipelines) != 2 || len(config.ReleaseEnvironments) != 2 || config.ReleaseTimeoutSeconds != 180 || config.ReleaseWebhookMaxAgeSeconds != 240 {
		t.Fatalf("unexpected stage 8D config: %#v", config)
	}
}

func TestLoadRejectsInvalidPricing(t *testing.T) {
	t.Setenv("EKBDA_LLM_INPUT_USD_PER_MILLION_TOKENS", "NaN")
	t.Setenv("EKBDA_LLM_OUTPUT_USD_PER_MILLION_TOKENS", "-1")
	config := Load()
	if config.LLMInputUSDPerMillionTokens != 0 || config.LLMOutputUSDPerMillionTokens != 0 {
		t.Fatalf("invalid pricing must be zero: %#v", config)
	}
}
