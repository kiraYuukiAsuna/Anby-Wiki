// Command api 是 HTTP API 入口，只负责装配，不含业务逻辑。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/aiconfig"
	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/collaboration"
	"github.com/anby/wiki/backend/internal/collection"
	"github.com/anby/wiki/backend/internal/component"
	"github.com/anby/wiki/backend/internal/dataset"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/importer"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/config"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/platform/logging"
	"github.com/anby/wiki/backend/internal/platform/observability"
	"github.com/anby/wiki/backend/internal/platform/ratelimit"
	"github.com/anby/wiki/backend/internal/platform/storage"
	"github.com/anby/wiki/backend/internal/projection"
	wikisearch "github.com/anby/wiki/backend/internal/search"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// 版本信息，由构建期 -ldflags 注入。
var version = "dev"

const serviceName = "wiki-api"

func main() {
	// 配置缺失不阻止启动：/readyz 降级报告 not_configured。
	cfg, cfgErr := config.Load()
	logger := logging.New(os.Stdout, cfg.LogLevel)
	if cfgErr != nil {
		if cfg.Env == "production" {
			logger.Error("production 配置无效，拒绝启动", slog.Any("error", cfgErr))
			os.Exit(1)
		}
		logger.Warn("配置不完整，就绪检查将降级", slog.Any("error", cfgErr))
	}
	metrics := observability.NewMetrics(serviceName)
	shutdownTracing, err := observability.InitTracing(context.Background(), observability.TracingConfig{
		Enabled: cfg.OTelEnabled, Endpoint: cfg.OTLPEndpoint, Insecure: cfg.OTLPInsecure,
		SampleRatio: cfg.OTelSampleRate, Service: serviceName, Version: version, Environment: cfg.Env,
	})
	if err != nil {
		logger.Error("初始化 tracing 失败", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			logger.Warn("关闭 tracing 失败", slog.Any("error", err))
		}
	}()

	allowDevActorHeader := cfg.AuthDevHeaderEnabled && cfg.Env != "production"
	deps := Deps{
		Service:             serviceName,
		Version:             version,
		Environment:         cfg.Env,
		AllowDevActorHeader: allowDevActorHeader,
		Metrics:             metrics,
		Checks: map[string]CheckFunc{
			"postgres": postgresCheck(cfg.DatabaseURL),
			"redis":    redisCheck(cfg.RedisURL),
		},
	}

	// Redis-backed limiter. The API is the only rate-limiting layer now that
	// no reverse proxy fronts it, so this is configured before serving.
	if cfg.RateLimitEnabled && strings.TrimSpace(cfg.RedisURL) != "" {
		redisOptions, redisErr := redis.ParseURL(cfg.RedisURL)
		if redisErr != nil {
			logger.Error("REDIS_URL 无法解析，限流未启用", slog.Any("error", redisErr))
			if cfg.Env == "production" {
				os.Exit(1)
			}
		} else {
			redisClient := redis.NewClient(redisOptions)
			defer func() {
				if err := redisClient.Close(); err != nil {
					logger.Warn("关闭 Redis 连接失败", slog.Any("error", err))
				}
			}()
			deps.RateLimiter = ratelimit.New(redisClient, time.Minute)
			deps.RateLimitConfig = RateLimitConfig{
				GeneralPerMinute: cfg.RateLimitGeneralPerMinute,
				AuthPerMinute:    cfg.RateLimitAuthPerMinute,
				UploadPerMinute:  cfg.RateLimitUploadPerMinute,
				TrustedProxies:   ratelimit.TrustedProxySet(cfg.TrustedProxyIPs),
			}
		}
	} else if cfg.Env == "production" && !cfg.RateLimitEnabled {
		logger.Warn("production 未启用限流，API 暴露在请求洪峰下")
	}

	// 读写 API 需要数据库：连接、装配领域服务并解析缓存默认站点 ID。
	// 未配置 DATABASE_URL 时降级为仅探针端点（与 /readyz 的 not_configured 语义一致）。
	var writeAPI *WriteAPI
	var readAPI *ReadAPI
	var historyAPI *HistoryAPI
	var projectionAPI *ProjectionAPI
	var searchAPI *SearchAPI
	var knowledgeReadAPI *KnowledgeReadAPI
	var governanceAPI *GovernanceAPI
	var importAPI *ImportAPI
	var authAPI *AuthAPI
	var collaborationAPI *CollaborationAPI
	var collectionAPI *CollectionAPI
	var assetAPI *AssetAPI
	var datasetAPI *DatasetAPI
	var componentAPI *ComponentAPI
	var sourceAPI *SourceAPI
	var aiConfigAPI *AIConfigAPI
	var pool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		var err error
		pool, err = db.Connect(context.Background(), cfg.DatabaseURL)
		if err != nil {
			logger.Error("连接数据库失败，无法装配 API", slog.Any("error", err))
			os.Exit(1)
		}
		defer pool.Close()
		searchBackend, backendErr := configuredSearchBackend(context.Background(), pool, cfg)
		if backendErr != nil {
			logger.Error("装配搜索后端失败", slog.Any("error", backendErr))
			os.Exit(1)
		}
		writeAPI, readAPI, historyAPI, projectionAPI, searchAPI, knowledgeReadAPI, governanceAPI, err = assembleAPIs(pool, searchBackend)
		if err != nil {
			logger.Error("装配 API 失败", slog.Any("error", err))
			os.Exit(1)
		}
		writeAPI.pages.WithPublishObserver(metrics)
		var objectStore storage.Store
		if cfg.S3Endpoint != "" && cfg.S3Bucket != "" &&
			cfg.S3AccessKey != "" && cfg.S3SecretKey != "" {
			objectStore = storage.NewS3Store(storage.S3Config{
				Endpoint: cfg.S3Endpoint, Region: cfg.S3Region,
				Bucket: cfg.S3Bucket, AccessKey: cfg.S3AccessKey,
				SecretKey: cfg.S3SecretKey,
			})
			writeAPI.pages.WithSnapshotStorage(page.SnapshotStorageConfig{
				Store: objectStore, Environment: cfg.Env,
				MaxBytes:  cfg.RevisionArchiveMaxBytes,
				Retention: cfg.RevisionArchiveRetention,
				BatchSize: cfg.RevisionArchiveBatchSize,
			})
		}
		governanceAPI.WithRevisionStorage(writeAPI.pages)
		importRepo := importer.NewRepository(pool)
		importAPI = NewImportAPI(importer.NewService(importRepo, db.NewTxManager(pool), id.NewGenerator()))
		authService := authdomain.NewService(pool, db.NewTxManager(pool), id.NewGenerator(), cfg.SessionTTL)
		deps.Authenticator = authdomain.NewAuthenticator(
			authService, cfg.SessionCookieName, allowDevActorHeader,
		)
		authAPI = NewAuthAPI(
			authService, cfg.SessionCookieName, cfg.SessionCookieSecure,
			cfg.AuthRegistrationEnabled,
		)
		collaborationService := collaboration.NewService(
			pool, db.NewTxManager(pool), id.NewGenerator(),
		)
		collaborationHub := collaboration.NewHub()
		collaborationAPI = NewCollaborationAPI(
			collaborationService,
			governance.NewAuthorizationService(pool),
			collaborationHub,
			writeAPI.wikiID,
			cfg.CollaborationOriginPatterns,
		)
		collectionAPI = NewCollectionAPI(
			collection.NewService(
				collection.NewRepository(pool), page.NewRepository(pool),
				db.NewTxManager(pool), id.NewGenerator(),
			),
			writeAPI.wikiID,
		)
		datasetAPI = NewDatasetAPI(
			dataset.NewService(
				dataset.NewRepository(pool), page.NewRepository(pool),
				db.NewTxManager(pool), id.NewGenerator(),
			),
			writeAPI.wikiID,
		)
		componentAPI = NewComponentAPI(
			component.NewService(
				component.NewRepository(pool), page.NewRepository(pool),
				db.NewTxManager(pool), id.NewGenerator(),
				component.NewKnowledgeRegistry(pool),
			),
		)
		sourceAPI = NewSourceAPI(evidence.NewService(
			evidence.NewRepository(pool), page.NewRepository(pool), nil, cfg.Env,
			db.NewTxManager(pool), id.NewGenerator(),
		))
		if strings.TrimSpace(cfg.AIConfigMasterKey) != "" && strings.TrimSpace(cfg.AIKernelInternalToken) != "" {
			aiConfigService, aiConfigErr := aiconfig.NewService(
				aiconfig.NewRepository(pool), db.NewTxManager(pool), id.NewGenerator(),
				governance.NewAuthorizationService(pool), cfg.AIConfigMasterKey,
			)
			if aiConfigErr != nil {
				logger.Error("装配 AI 配置服务失败", slog.Any("error", aiConfigErr))
				os.Exit(1)
			}
			kernelProvider, kernelErr := ai.NewSemanticKernelProvider(
				cfg.AIKernelURL, cfg.AIKernelInternalToken,
				ai.SemanticKernelConfigResolverFunc(func(ctx context.Context) (*ai.SemanticKernelConfig, error) {
					runtime, err := aiConfigService.Runtime(ctx, writeAPI.wikiID)
					if err != nil {
						return nil, err
					}
					return &ai.SemanticKernelConfig{
						Provider: runtime.Provider, BaseURL: runtime.BaseURL, APIKey: runtime.APIKey,
						Model: runtime.Model, ResponseFormat: runtime.ResponseFormat,
						RequestTimeoutSeconds: runtime.RequestTimeoutSeconds,
						MaxAttempts:           runtime.MaxAttempts,
					}, nil
				}), nil,
			)
			if kernelErr != nil {
				logger.Error("装配 Semantic Kernel 客户端失败", slog.Any("error", kernelErr))
				os.Exit(1)
			}
			aiConfigAPI = NewAIConfigAPI(aiConfigService, kernelProvider, writeAPI.wikiID)
		}
		governanceAPI.WithWorkingDocumentMerge(
			governance.NewMergeWorkingDocumentService(
				governanceAPI.repo,
				governance.NewPagePatchEngine(),
				governanceAPI.apply.Conflicts(),
				collaborationService,
				db.NewTxManager(pool),
				id.NewGenerator(),
			).WithAuthorization(governance.NewAuthorizationService(pool)),
			collaborationHub,
		)
		governanceAPI.WithConflictResolution(
			governance.NewConflictResolutionService(
				governanceAPI.repo, db.NewTxManager(pool), id.NewGenerator(),
			).WithAuthorization(governance.NewAuthorizationService(pool)),
		)
		if objectStore != nil {
			pageRepo := page.NewRepository(pool)
			wikiID, wikiErr := pageRepo.GetWikiIDBySiteKey(context.Background(), nil, "default")
			if wikiErr != nil {
				logger.Error("装配上传导入失败", slog.Any("error", wikiErr))
				os.Exit(1)
			}
			evidenceService := evidence.NewService(evidence.NewRepository(pool), pageRepo, objectStore,
				cfg.Env, db.NewTxManager(pool), id.NewGenerator())
			importAPI.WithUploads(evidenceService, wikiID)
			assetAPI = NewAssetAPI(evidenceService, wikiID)
		}
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.APIPort),
		Handler:           NewRouter(logger, deps, writeAPI, readAPI, historyAPI, projectionAPI, searchAPI, knowledgeReadAPI, governanceAPI, importAPI, authAPI, collaborationAPI, collectionAPI, assetAPI, datasetAPI, componentAPI, sourceAPI, aiConfigAPI),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// 监听 SIGINT/SIGTERM 触发优雅退出。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("api 启动",
			slog.String("addr", srv.Addr),
			slog.String("env", cfg.Env),
			slog.String("version", version),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http 服务异常退出", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("收到退出信号，开始优雅退出")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("优雅退出失败", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("api 已退出")
}

// assembleAPIs 在连接池上装配 Page、Knowledge、Evidence 领域服务与 HTTP API，并解析缓存默认站点
// （种子里 site_key='default'）的 ID——M1 单站点，所有 API 都落在该站点。
func assembleAPIs(pool *pgxpool.Pool, searchBackends ...wikisearch.SearchAdapter) (*WriteAPI, *ReadAPI, *HistoryAPI, *ProjectionAPI, *SearchAPI, *KnowledgeReadAPI, *GovernanceAPI, error) {
	repo := page.NewRepository(pool)
	txm := db.NewTxManager(pool)
	ids := id.NewGenerator()
	svc := page.NewService(repo, txm, ids)
	wikiID, err := repo.GetWikiIDBySiteKey(context.Background(), nil, "default")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("解析默认站点失败: %w", err)
	}
	evidenceRepo := evidence.NewRepository(pool)
	evidenceService := evidence.NewService(evidenceRepo, repo, nil, "development", txm, ids)
	knowledgeRepo := knowledge.NewRepository(pool)
	knowledgeService := knowledge.NewService(knowledgeRepo, repo, txm, ids).
		WithCitationChecker(evidenceRepo)
	factConsistency := knowledge.NewFactConsistencyService(
		knowledgeRepo, txm, ids,
	)
	governanceRepo := governance.NewRepository(pool)
	proposalService := governance.NewService(governanceRepo, txm, ids)
	pagePatch := governance.NewPagePatchEngine()
	knowledgePatch := governance.NewKnowledgePatchEngine(knowledgeService)
	collectionService := collection.NewService(
		collection.NewRepository(pool), repo, txm, ids,
	)
	authorization := governance.NewAuthorizationService(pool)
	aiTrustService := governance.NewAITrustService(
		governanceRepo, authorization, txm, ids,
	)
	conflictService := governance.NewConflictService(governanceRepo, svc, knowledgeService, txm, ids)
	reviewService := governance.NewReviewService(governanceRepo, txm, ids,
		governance.NewRiskEvaluator(knowledgeService)).
		WithAuthorization(authorization).
		WithAITrust(aiTrustService)
	applyService := governance.NewApplyService(governanceRepo, svc, pagePatch, knowledgePatch,
		conflictService, txm, ids).
		WithAuthorization(authorization).
		WithCollections(collectionService)
	rollbackService := governance.NewRollbackService(governanceRepo, svc, knowledgePatch, txm, ids).
		WithAuthorization(authorization).
		WithCollections(collectionService)
	governanceAPI := NewGovernanceAPI(proposalService, governanceRepo,
		governance.NewPreviewService(governanceRepo, svc, pagePatch), reviewService,
		applyService, rollbackService).WithBulkReview(
		governance.NewBulkReviewService(governanceRepo, applyService, txm, ids).
			WithAuthorization(authorization).
			WithWikiID(wikiID),
	).WithAudit(governance.NewAuditService(
		governanceRepo, authorization, txm, ids, wikiID,
	)).WithAuthorization(authorization).WithAITrust(aiTrustService).WithProtection(
		governance.NewProtectionService(
			governanceRepo, authorization, txm, ids,
		),
		wikiID,
	)
	searchBackend := wikisearch.SearchAdapter(wikisearch.NewPostgresAdapter(pool))
	if len(searchBackends) > 0 && searchBackends[0] != nil {
		searchBackend = searchBackends[0]
	}
	searchAPI := NewSearchAPI(searchBackend, wikiID)
	if _, postgres := searchBackend.(*wikisearch.PostgresAdapter); postgres {
		searchAPI.WithTitleAliasFallback(wikisearch.NewPageAdapter(svc))
	}
	return NewWriteAPI(svc, wikiID).
			WithAuthorization(authorization).
			WithCollaborationPublisher(collaboration.NewPublisher(txm, ids, svc)),
		NewReadAPI(svc, wikiID, projection.NewQueries(pool)).
			WithDynamicRenderer(projection.NewDynamicHTMLRenderer(pool)),
		NewHistoryAPI(svc, wikiID),
		NewProjectionAPI(svc, projection.NewQueries(pool), wikiID),
		searchAPI,
		NewKnowledgeReadAPI(knowledgeService, evidenceService, wikiID).
			WithMergeAuthorization(authorization).
			WithFactConsistency(factConsistency).
			WithEntityGraph(projection.NewEntityGraphService(pool)),
		governanceAPI, nil
}

func configuredSearchBackend(ctx context.Context, pool *pgxpool.Pool, cfg config.Config) (wikisearch.SearchAdapter, error) {
	adapter, err := wikisearch.NewBackend(pool, wikisearch.BackendConfig{
		Backend: cfg.SearchBackend, MeiliURL: cfg.MeiliURL, MeiliAPIKey: cfg.MeiliAPIKey,
		MeiliIndex: cfg.MeiliIndex, MeiliTimeout: cfg.MeiliTimeout,
		MeiliTaskTimeout: cfg.MeiliTaskTimeout, SemanticEnabled: cfg.SearchSemanticEnabled,
		MeiliEmbedderName: cfg.MeiliEmbedderName, MeiliEmbedderSource: cfg.MeiliEmbedderSource,
		MeiliEmbedderModel: cfg.MeiliEmbedderModel, MeiliEmbedderAPIKey: cfg.MeiliEmbedderAPIKey,
	})
	if err != nil {
		return nil, err
	}
	if meili, ok := adapter.(*wikisearch.MeilisearchAdapter); ok {
		if err := meili.EnsureIndex(ctx); err != nil {
			return nil, err
		}
	}
	return adapter, nil
}

// postgresCheck 构造 PostgreSQL 就绪检查；未配置时返回 nil（降级 not_configured）。
// pgxpool.New 惰性建连，不会在启动时阻塞。
func postgresCheck(databaseURL string) CheckFunc {
	if databaseURL == "" {
		return nil
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil
	}
	return func(ctx context.Context) error {
		return db.Ping(ctx, pool)
	}
}

// redisCheck 构造 Redis 就绪检查（TCP 拨号探测，P0 不引入 Redis 客户端）。
func redisCheck(redisURL string) CheckFunc {
	if redisURL == "" {
		return nil
	}
	u, err := url.Parse(redisURL)
	if err != nil || u.Host == "" {
		return nil
	}
	return func(ctx context.Context) error {
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", u.Host)
		if err != nil {
			return fmt.Errorf("redis 不可达: %w", err)
		}
		return conn.Close()
	}
}
