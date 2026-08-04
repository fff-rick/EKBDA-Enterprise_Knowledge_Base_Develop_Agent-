package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ekbda/internal/access"
	"ekbda/internal/agentquality"
	"ekbda/internal/agenttask"
	"ekbda/internal/answer"
	"ekbda/internal/auth"
	"ekbda/internal/config"
	"ekbda/internal/development"
	"ekbda/internal/embedding"
	"ekbda/internal/evaluation"
	"ekbda/internal/httpapi"
	"ekbda/internal/ingestion"
	"ekbda/internal/initiative"
	"ekbda/internal/knowledge"
	"ekbda/internal/planning"
	"ekbda/internal/release"
	"ekbda/internal/repositorysync"
	"ekbda/internal/reranking"
	"ekbda/internal/standards"
	"ekbda/internal/workspace"
)

func main() {
	cfg := config.Load()
	startupContext, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	repository, closeRepository, err := buildRepository(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize knowledge repository", "error", err)
		os.Exit(1)
	}
	defer closeRepository()
	jobStore, closeJobStore, err := buildJobStore(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize ingestion job store", "error", err)
		os.Exit(1)
	}
	defer closeJobStore()
	traceStore, closeTraceStore, err := buildTraceStore(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize answer trace store", "error", err)
		os.Exit(1)
	}
	defer closeTraceStore()
	evaluationStore, closeEvaluationStore, err := buildEvaluationStore(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize evaluation store", "error", err)
		os.Exit(1)
	}
	defer closeEvaluationStore()
	standardsStore, closeStandardsStore, err := buildStandardsStore(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize standards store", "error", err)
		os.Exit(1)
	}
	defer closeStandardsStore()
	workspaceStore, closeWorkspaceStore, err := buildWorkspaceStore(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize workspace store", "error", err)
		os.Exit(1)
	}
	defer closeWorkspaceStore()
	accessStore, closeAccessStore, err := buildAccessStore(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize project access store", "error", err)
		os.Exit(1)
	}
	defer closeAccessStore()
	repositorySyncStore, closeRepositorySyncStore, err := buildRepositorySyncStore(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize repository knowledge sync store", "error", err)
		os.Exit(1)
	}
	defer closeRepositorySyncStore()
	planningStore, closePlanningStore, err := buildPlanningStore(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize planning store", "error", err)
		os.Exit(1)
	}
	defer closePlanningStore()
	initiativeStore, closeInitiativeStore, err := buildInitiativeStore(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize project package store", "error", err)
		os.Exit(1)
	}
	defer closeInitiativeStore()
	agentTaskStore, closeAgentTaskStore, err := buildAgentTaskStore(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize agent task store", "error", err)
		os.Exit(1)
	}
	defer closeAgentTaskStore()
	developmentStore, closeDevelopmentStore, err := buildDevelopmentStore(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize development session store", "error", err)
		os.Exit(1)
	}
	defer closeDevelopmentStore()
	releaseStore, closeReleaseStore, err := buildReleaseStore(startupContext, cfg)
	if err != nil {
		cancelStartup()
		slog.Error("initialize release request store", "error", err)
		os.Exit(1)
	}
	defer closeReleaseStore()
	accessService, err := access.New(accessStore, cfg.ProjectAuthorizationMode)
	if err != nil {
		cancelStartup()
		slog.Error("initialize project authorization", "error", err)
		os.Exit(1)
	}
	authenticator, err := buildAuthenticator(startupContext, cfg)
	cancelStartup()
	if err != nil {
		slog.Error("initialize request authentication", "error", err)
		os.Exit(1)
	}

	embeddingProvider, err := buildEmbeddingProvider(cfg)
	if err != nil {
		slog.Error("initialize embedding provider", "error", err)
		os.Exit(1)
	}
	reranker, err := buildReranker(cfg)
	if err != nil {
		slog.Error("initialize reranker", "error", err)
		os.Exit(1)
	}
	service := knowledge.NewService(repository, embeddingProvider, reranker)
	answerProvider, err := buildAnswerProvider(cfg)
	if err != nil {
		slog.Error("initialize answer provider", "error", err)
		os.Exit(1)
	}
	answerService := answer.NewService(service, answerProvider, traceStore, answer.Pricing{
		InputUSDPerMillionTokens:  cfg.LLMInputUSDPerMillionTokens,
		OutputUSDPerMillionTokens: cfg.LLMOutputUSDPerMillionTokens,
	})
	evaluationRunner := evaluation.NewRunner(answerService)
	evaluationService := evaluation.NewService(evaluationRunner, evaluationStore)
	evaluationService.Start()
	defer evaluationService.Close()
	standardsService := standards.NewService(standardsStore)
	workspaceService, err := workspace.New(cfg.WorkspaceRoot, standardsService, workspaceStore)
	if err != nil {
		slog.Error("initialize Git workspace service", "error", err)
		os.Exit(1)
	}
	repositorySyncService := repositorysync.New(workspaceService, service, repositorySyncStore)
	planningProvider, err := buildPlanningProvider(cfg)
	if err != nil {
		slog.Error("initialize planning provider", "error", err)
		os.Exit(1)
	}
	planningService := planning.NewService(planningStore, planningProvider, service, standardsService, workspaceService, repositorySyncService)
	initiativeProvider, err := buildInitiativeProvider(cfg)
	if err != nil {
		slog.Error("initialize project package provider", "error", err)
		os.Exit(1)
	}
	initiativeService := initiative.NewService(initiativeStore, initiativeProvider, planningService)
	secretScanner, err := buildDevelopmentSecretScanner(cfg)
	if err != nil {
		slog.Error("initialize enterprise secret scanner", "error", err)
		os.Exit(1)
	}
	developmentRunner, err := buildDevelopmentRunner(cfg, standardsService, secretScanner)
	if err != nil {
		slog.Error("initialize controlled development runner", "error", err)
		os.Exit(1)
	}
	developmentDeliverer, err := buildDevelopmentDeliverer(cfg, secretScanner)
	if err != nil {
		slog.Error("initialize controlled development delivery", "error", err)
		os.Exit(1)
	}
	developmentService := development.NewServiceWithDelivery(developmentStore, initiativeService, workspaceService, developmentRunner, developmentDeliverer)
	releaseConnector, err := release.NewHTTPConnector(release.ConnectorConfig{Enabled: cfg.ReleaseEnabled, BaseURL: cfg.ReleaseProviderBaseURL, Token: cfg.ReleaseProviderToken, Timeout: time.Duration(cfg.ReleaseTimeoutSeconds) * time.Second})
	if err != nil {
		slog.Error("initialize controlled CI/CD connector", "error", err)
		os.Exit(1)
	}
	releaseReader := release.DevelopmentReaderFunc(func(ctx context.Context, id string) (release.DevelopmentSession, error) {
		session, err := developmentService.Get(ctx, id)
		if err != nil {
			return release.DevelopmentSession{}, err
		}
		result := release.DevelopmentSession{ID: session.ID, Project: session.Project, Repository: session.Repository, Status: session.Status}
		if session.Delivery != nil {
			result.DeliveryStatus = session.Delivery.Status
			result.Commit = session.Delivery.Commit
			result.PullRequestURL = session.Delivery.PullRequestURL
		}
		return result, nil
	})
	releaseService, err := release.NewService(releaseStore, releaseReader, releaseConnector, cfg.ReleasePipelines, cfg.ReleaseEnvironments)
	if err != nil {
		slog.Error("initialize release orchestration", "error", err)
		os.Exit(1)
	}
	var releaseWebhook *release.WebhookVerifier
	var codeWebhook *release.WebhookVerifier
	if cfg.ReleaseEnabled {
		releaseWebhook, err = release.NewWebhookVerifier(cfg.ReleaseWebhookSecret, time.Duration(cfg.ReleaseWebhookMaxAgeSeconds)*time.Second)
		if err != nil {
			slog.Error("initialize release webhook verifier", "error", err)
			os.Exit(1)
		}
		codeWebhook, err = release.NewWebhookVerifier(cfg.ReleaseCodeWebhookSecret, time.Duration(cfg.ReleaseWebhookMaxAgeSeconds)*time.Second)
		if err != nil {
			slog.Error("initialize code platform webhook verifier", "error", err)
			os.Exit(1)
		}
	}
	recoveryContext, cancelRecovery := context.WithTimeout(context.Background(), 30*time.Second)
	if err := developmentService.Recover(recoveryContext); err != nil {
		cancelRecovery()
		slog.Error("recover controlled development executions", "error", err)
		os.Exit(1)
	}
	cancelRecovery()
	agentTaskService := agenttask.NewService(agentTaskStore, agenttask.Pricing{
		InputUSDPerMillionTokens:  cfg.LLMInputUSDPerMillionTokens,
		OutputUSDPerMillionTokens: cfg.LLMOutputUSDPerMillionTokens,
	}, time.Duration(cfg.AgentTaskTimeoutSeconds)*time.Second)
	if err := agentTaskService.Register(agenttask.KindRoleReview, func(ctx context.Context, task agenttask.Task) (agenttask.ExecutionResult, error) {
		var input agenttask.RoleReviewInput
		if err := json.Unmarshal(task.Input, &input); err != nil {
			return agenttask.ExecutionResult{}, err
		}
		session, err := planningService.SubmitRoleReviews(ctx, input.SessionID, input.Revision, task.TriggeredBy, input.Roles, input.GovernanceOverride)
		if err != nil {
			return agenttask.ExecutionResult{}, err
		}
		return agenttask.ExecutionResult{ResourceID: session.ID, Quality: agentquality.RoleReview(session)}, nil
	}); err != nil {
		slog.Error("register role-review agent task", "error", err)
		os.Exit(1)
	}
	if err := agentTaskService.Register(agenttask.KindProjectPackage, func(ctx context.Context, task agenttask.Task) (agenttask.ExecutionResult, error) {
		var input agenttask.ProjectPackageInput
		if err := json.Unmarshal(task.Input, &input); err != nil {
			return agenttask.ExecutionResult{}, err
		}
		projectPackage, err := initiativeService.Create(ctx, initiative.CreateInput{SessionID: input.SessionID, Name: input.Name, ChangeSummary: input.ChangeSummary}, task.TriggeredBy)
		if err != nil {
			return agenttask.ExecutionResult{}, err
		}
		return agenttask.ExecutionResult{ResourceID: projectPackage.ID, Quality: agentquality.ProjectPackage(projectPackage)}, nil
	}); err != nil {
		slog.Error("register project-package agent task", "error", err)
		os.Exit(1)
	}
	agentTaskService.Start()
	defer agentTaskService.Close()
	ingestionService := ingestion.New(cfg.ImportRoot, service, jobStore)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewWithReleaseWebhooks(service, ingestionService, answerService, evaluationService, standardsService, workspaceService, accessService, repositorySyncService, planningService, initiativeService, agentTaskService, developmentService, releaseService, codeWebhook, releaseWebhook, authenticator),
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdownSignals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-shutdownSignals.Done():
				return
			case <-ticker.C:
				recoveryContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				err := developmentService.Recover(recoveryContext)
				cancel()
				if err != nil {
					slog.Error("recover stale controlled development execution", "error", err)
				}
			}
		}
	}()

	go func() {
		slog.Info("EKBDA API started", "address", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	<-shutdownSignals.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown failed", "error", err)
	}
}

func buildDevelopmentStore(ctx context.Context, cfg config.Config) (development.Store, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return development.NewMemoryStore(), func() {}, nil
	case "postgres":
		store, err := development.NewPostgresStore(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			if err := store.Close(); err != nil {
				slog.Error("close development PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}

func buildReleaseStore(ctx context.Context, cfg config.Config) (release.Store, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return release.NewMemoryStore(), func() {}, nil
	case "postgres":
		store, err := release.NewPostgresStore(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			if err := store.Close(); err != nil {
				slog.Error("close release PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}

func buildDevelopmentSecretScanner(cfg config.Config) (development.SecretScanner, error) {
	required := cfg.DevelopmentDeliveryEnabled || (cfg.DevelopmentExecutionEnabled && cfg.DevelopmentExecutionDriver == "container")
	if !required {
		return nil, nil
	}
	return development.NewExternalSecretScanner(
		cfg.DevelopmentSecretScannerName,
		cfg.DevelopmentSecretScannerBinary,
		cfg.DevelopmentSecretScannerArguments,
		cfg.DevelopmentSecretScannerEnv,
		time.Duration(cfg.DevelopmentExecutionTimeoutSeconds)*time.Second,
	)
}

func buildDevelopmentRunner(cfg config.Config, standardsService *standards.Service, scanner development.SecretScanner) (development.Runner, error) {
	timeout := time.Duration(cfg.DevelopmentExecutionTimeoutSeconds) * time.Second
	if !cfg.DevelopmentExecutionEnabled {
		return development.NewLocalRunner(false, "", "", standardsService, timeout)
	}
	switch cfg.DevelopmentExecutionDriver {
	case "local":
		return development.NewLocalRunner(true, cfg.WorkspaceRoot, cfg.DevelopmentExecutionRoot, standardsService, timeout)
	case "container":
		return development.NewContainerRunner(true, cfg.WorkspaceRoot, cfg.DevelopmentExecutionRoot, standardsService, timeout, scanner, development.ContainerConfig{
			Binary: cfg.DevelopmentContainerBinary, Image: cfg.DevelopmentContainerImage,
			CPUs: cfg.DevelopmentContainerCPUs, Memory: cfg.DevelopmentContainerMemory,
			PIDs: cfg.DevelopmentContainerPIDs, WritableTmpSize: cfg.DevelopmentContainerTmpSize,
			User: cfg.DevelopmentContainerUser, GoModCache: cfg.DevelopmentContainerGoModCache,
		})
	default:
		return nil, development.ErrInvalidExecutionConfig
	}
}

func buildDevelopmentDeliverer(cfg config.Config, scanner development.SecretScanner) (development.Deliverer, error) {
	if !cfg.DevelopmentDeliveryEnabled {
		return development.NewGitDeliverer(development.DeliveryConfig{}, nil, nil)
	}
	timeout := time.Duration(cfg.DevelopmentDeliveryTimeoutSeconds) * time.Second
	publisher, err := development.NewGitHubCLIPublisher(cfg.DevelopmentPRBinary, timeout)
	if err != nil {
		return nil, err
	}
	return development.NewGitDeliverer(development.DeliveryConfig{
		Enabled: true, WorkspaceRoot: cfg.WorkspaceRoot, DeliveryRoot: cfg.DevelopmentDeliveryRoot,
		Remote: cfg.DevelopmentDeliveryRemote, AuthorName: cfg.DevelopmentDeliveryAuthorName,
		AuthorEmail: cfg.DevelopmentDeliveryAuthorEmail, Username: cfg.DevelopmentDeliveryUsername,
		Token: cfg.DevelopmentDeliveryToken, Timeout: timeout,
	}, scanner, publisher)
}

func buildAgentTaskStore(ctx context.Context, cfg config.Config) (agenttask.Store, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return agenttask.NewMemoryStore(), func() {}, nil
	case "postgres":
		store, err := agenttask.NewPostgresStore(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			if err := store.Close(); err != nil {
				slog.Error("close agent task PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}

func buildInitiativeStore(ctx context.Context, cfg config.Config) (initiative.Store, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return initiative.NewMemoryStore(), func() {}, nil
	case "postgres":
		store, err := initiative.NewPostgresStore(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			if err := store.Close(); err != nil {
				slog.Error("close project package PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}

func buildInitiativeProvider(cfg config.Config) (initiative.Provider, error) {
	switch cfg.PlannerProvider {
	case "local":
		return initiative.NewLocalProvider(), nil
	case "openai-compatible":
		return initiative.NewOpenAICompatibleProvider(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, 60*time.Second)
	default:
		return nil, errors.New("unsupported project package provider: " + cfg.PlannerProvider)
	}
}

func buildPlanningStore(ctx context.Context, cfg config.Config) (planning.Store, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return planning.NewMemoryStore(), func() {}, nil
	case "postgres":
		store, err := planning.NewPostgresStore(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			if err := store.Close(); err != nil {
				slog.Error("close planning PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}

func buildPlanningProvider(cfg config.Config) (planning.Provider, error) {
	switch cfg.PlannerProvider {
	case "local":
		return planning.NewLocalProvider(), nil
	case "openai-compatible":
		return planning.NewOpenAICompatibleProvider(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, 60*time.Second)
	default:
		return nil, errors.New("unsupported planning provider: " + cfg.PlannerProvider)
	}
}

func buildRepositorySyncStore(ctx context.Context, cfg config.Config) (repositorysync.Store, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return repositorysync.NewMemoryStore(), func() {}, nil
	case "postgres":
		store, err := repositorysync.NewPostgresStore(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			if err := store.Close(); err != nil {
				slog.Error("close repository sync PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}

func buildAccessStore(ctx context.Context, cfg config.Config) (access.Store, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return access.NewMemoryStore(), func() {}, nil
	case "postgres":
		store, err := access.NewPostgresStore(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			if err := store.Close(); err != nil {
				slog.Error("close access PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}

func buildWorkspaceStore(ctx context.Context, cfg config.Config) (workspace.Store, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return workspace.NewMemoryStore(), func() {}, nil
	case "postgres":
		store, err := workspace.NewPostgresStore(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			if err := store.Close(); err != nil {
				slog.Error("close workspace PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}

func buildStandardsStore(ctx context.Context, cfg config.Config) (standards.Store, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return standards.NewMemoryStore(), func() {}, nil
	case "postgres":
		store, err := standards.NewPostgresStore(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			if err := store.Close(); err != nil {
				slog.Error("close standards PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}

func buildAuthenticator(ctx context.Context, cfg config.Config) (auth.Authenticator, error) {
	switch cfg.AuthMode {
	case "dev_headers":
		return auth.NewDevHeaders(), nil
	case "jwt":
		return auth.NewJWTAuthenticator(ctx, auth.JWTConfig{
			Issuer:            cfg.JWTIssuer,
			Audience:          cfg.JWTAudience,
			JWKSURL:           cfg.JWTJWKSURL,
			UserIDClaim:       cfg.JWTUserIDClaim,
			RolesClaim:        cfg.JWTRolesClaim,
			ClockSkew:         time.Duration(cfg.JWTClockSkewSeconds) * time.Second,
			AllowInsecureHTTP: cfg.JWTAllowInsecureHTTP,
		})
	default:
		return nil, errors.New("unsupported authentication mode: " + cfg.AuthMode)
	}
}

func buildReranker(cfg config.Config) (reranking.Provider, error) {
	local := reranking.NewLocal()
	switch cfg.RerankProvider {
	case "local":
		return local, nil
	case "http":
		provider, err := reranking.NewHTTP(cfg.RerankBaseURL, cfg.RerankAPIKey, cfg.RerankModel, 30*time.Second)
		if err != nil {
			return nil, err
		}
		return reranking.NewFallback(provider, local), nil
	default:
		return nil, errors.New("unsupported rerank provider: " + cfg.RerankProvider)
	}
}

func buildEvaluationStore(ctx context.Context, cfg config.Config) (evaluation.Store, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return evaluation.NewMemoryStore(), func() {}, nil
	case "postgres":
		store, err := evaluation.NewPostgresStore(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			if err := store.Close(); err != nil {
				slog.Error("close evaluation PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}

func buildTraceStore(ctx context.Context, cfg config.Config) (answer.TraceStore, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return answer.NewMemoryTraceStore(), func() {}, nil
	case "postgres":
		store, err := answer.NewPostgresTraceStore(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			if err := store.Close(); err != nil {
				slog.Error("close answer trace PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}

func buildAnswerProvider(cfg config.Config) (answer.Provider, error) {
	switch cfg.AnswerProvider {
	case "local":
		return answer.NewLocalExtractive(), nil
	case "openai-compatible":
		return answer.NewOpenAICompatible(
			cfg.LLMBaseURL,
			cfg.LLMAPIKey,
			cfg.LLMModel,
			60*time.Second,
		)
	default:
		return nil, errors.New("unsupported answer provider: " + cfg.AnswerProvider)
	}
}

func buildEmbeddingProvider(cfg config.Config) (embedding.Provider, error) {
	switch cfg.EmbeddingProvider {
	case "local":
		return embedding.NewLocalHash(cfg.EmbeddingDimension), nil
	case "openai-compatible":
		return embedding.NewOpenAICompatible(
			cfg.EmbeddingBaseURL,
			cfg.EmbeddingAPIKey,
			cfg.EmbeddingModel,
			30*time.Second,
		)
	default:
		return nil, errors.New("unsupported embedding provider: " + cfg.EmbeddingProvider)
	}
}

func buildJobStore(ctx context.Context, cfg config.Config) (ingestion.JobStore, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return ingestion.NewMemoryJobStore(), func() {}, nil
	case "postgres":
		store, err := ingestion.NewPostgresJobStore(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			if err := store.Close(); err != nil {
				slog.Error("close ingestion PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}

func buildRepository(ctx context.Context, cfg config.Config) (knowledge.Repository, func(), error) {
	switch cfg.StorageDriver {
	case "memory":
		return knowledge.NewMemoryRepository(), func() {}, nil
	case "postgres":
		repository, err := knowledge.NewPostgresRepository(ctx, cfg.PostgresDSN, cfg.EmbeddingDimension)
		if err != nil {
			return nil, nil, err
		}
		return repository, func() {
			if err := repository.Close(); err != nil {
				slog.Error("close PostgreSQL connection", "error", err)
			}
		}, nil
	default:
		return nil, nil, errors.New("unsupported storage driver: " + cfg.StorageDriver)
	}
}
