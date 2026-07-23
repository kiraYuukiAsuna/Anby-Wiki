// Command worker 是异步 Worker 入口（Outbox、Projection、导入）。
// 装配 M3-T01 Outbox 消费框架 + M3-T02 Projection 通用框架：
// page.revision_published 分发到 Builder 注册表（带版本防护并写 projection_state）；
// M3-T03 注册 page_links / document_outline 投影 Builder，M3-T05 注册 rendered_page；
// M3-T04 注册未解析链接 Resolver：消费 page.created / page.renamed，
// 并把 ResolvePageLinks 作为 hook 挂在 page.revision_published 分发之后（乱序兜底）。
//
// 重建命令（执行后退出，设计 §15/§16 投影可丢弃可重建）：
//
//	worker -rebuild-page <page-uuid>   重建指定页面的全部已注册投影
//	worker -rebuild-all                全量重放：分页扫描全部活页面逐页重建
//	worker -metrics                    输出一次积压/延迟/失败指标后退出
//	worker -check-consistency          抽检投影状态与页面 Current 是否一致后退出
//	worker -replay-dead                将全部死信重置为 pending 后退出
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/linkhealth"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/config"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/platform/logging"
	"github.com/anby/wiki/backend/internal/platform/observability"
	"github.com/anby/wiki/backend/internal/projection"
	wikisearch "github.com/anby/wiki/backend/internal/search"
)

// 版本信息，由构建期 -ldflags 注入。
var version = "dev"

const serviceName = "wiki-worker"

// 退出码：0 成功；1 运行错误；2 参数错误。
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run 解析参数并执行，返回进程退出码（拆出以便测试重建命令路径）。
func run(args []string) int {
	fs := flag.NewFlagSet(serviceName, flag.ContinueOnError)
	rebuildPage := fs.String("rebuild-page", "", "重建指定 Page ID 的全部已注册投影后退出")
	rebuildAll := fs.Bool("rebuild-all", false, "全量重放：重建全部活页面的投影后退出")
	metrics := fs.Bool("metrics", false, "输出一次 Outbox/Projection 运维指标后退出")
	checkConsistency := fs.Bool("check-consistency", false, "抽检投影状态与当前 Revision 一致性后退出")
	sampleSize := fs.Int("sample-size", 100, "一致性抽检页面数（1..10000）")
	replayDead := fs.Bool("replay-dead", false, "将全部 Outbox 死信重置为 pending 后退出")
	checkExternalLinks := fs.Bool("check-external-links", false, "执行一批到期外链健康检查后退出")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	modeCount := 0
	for _, enabled := range []bool{*rebuildAll, *rebuildPage != "", *metrics, *checkConsistency, *replayDead, *checkExternalLinks} {
		if enabled {
			modeCount++
		}
	}
	if modeCount > 1 {
		return exitUsage
	}
	if *sampleSize <= 0 || *sampleSize > 10_000 {
		return exitUsage
	}

	cfg, err := config.Load()
	logger := logging.New(os.Stdout, cfg.LogLevel)
	if err != nil {
		if cfg.Env == "production" {
			logger.Error("production 配置无效，拒绝启动", slog.Any("error", err))
			return exitError
		}
		logger.Warn("配置不完整，Worker 以降级模式运行", slog.Any("error", err))
	}
	metricsRegistry := observability.NewMetrics(serviceName)
	shutdownTracing, err := observability.InitTracing(context.Background(), observability.TracingConfig{
		Enabled: cfg.OTelEnabled, Endpoint: cfg.OTLPEndpoint, Insecure: cfg.OTLPInsecure,
		SampleRatio: cfg.OTelSampleRate, Service: serviceName, Version: version, Environment: cfg.Env,
	})
	if err != nil {
		logger.Error("初始化 tracing 失败", slog.Any("error", err))
		return exitError
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			logger.Warn("关闭 tracing 失败", slog.Any("error", err))
		}
	}()

	// 阻塞等待 SIGINT/SIGTERM，优雅退出（重建命令路径不等待信号，执行完即返回）。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.DatabaseURL == "" {
		if modeCount > 0 {
			logger.Error("未配置 DATABASE_URL，无法执行 Worker 运维命令")
			return exitError
		}
		// 未配置 DATABASE_URL 时降级：Outbox 消费停用，空转等待退出信号（与 cmd/api 风格一致）。
		logger.Warn("未配置 DATABASE_URL，Outbox 消费停用，worker 空转")
		<-ctx.Done()
		logger.Info("收到退出信号，worker 已退出")
		return exitOK
	}

	pool, err := db.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("连接数据库失败", slog.Any("error", err))
		return exitError
	}
	defer pool.Close()

	searchBackend, err := configuredWorkerSearchBackend(ctx, pool, cfg)
	if err != nil {
		logger.Error("装配搜索后端失败", slog.Any("error", err))
		return exitError
	}
	registry := projection.NewRegistry()
	searchBuilder := registerBuilders(registry, pool, logger, searchBackend)

	// 重建命令模式：执行后退出。
	if *rebuildAll || *rebuildPage != "" {
		return runRebuild(ctx, logger, pool, registry, *rebuildPage, *rebuildAll)
	}
	if *metrics {
		return runMetrics(ctx, logger, pool)
	}
	if *checkConsistency {
		return runConsistencyCheck(ctx, logger, pool, registry, *sampleSize)
	}
	if *replayDead {
		count, err := projection.ReplayDead(ctx, pool)
		if err != nil {
			logger.Error("死信重放失败", slog.Any("error", err))
			return exitError
		}
		logger.Info("死信已重置为 pending", slog.Int64("replayed", count))
		return exitOK
	}
	linkHealthRunner := linkhealth.NewRunner(pool, nil, logger)
	if *checkExternalLinks {
		report, err := linkHealthRunner.RunOnce(ctx, 20)
		if err != nil {
			logger.Error("外链健康检查失败", slog.Any("error", err))
			return exitError
		}
		logger.Info("外链健康检查完成",
			slog.Int("claimed", report.Claimed),
			slog.Int("ok", report.OK),
			slog.Int("redirect", report.Redirect),
			slog.Int("broken", report.Broken),
			slog.Int("blocked", report.Blocked),
			slog.Int("proposals", report.Proposals))
		return exitOK
	}

	// 常驻消费模式。
	logger.Info("worker 启动",
		slog.String("service", serviceName),
		slog.String("env", cfg.Env),
		slog.String("version", version),
		slog.Int("registered_builders", registry.Len()),
	)

	consumer := projection.New(pool, projection.Config{Logger: logger})
	// M3-T04 未解析链接 Resolver：page.created / page.renamed 直接消费；
	// page.revision_published 在 Builder 分发成功后追加 ResolvePageLinks hook，
	// 兜底 created/renamed 与 published 事件的乱序到达（最终一致）。
	resolver := projection.NewLinkResolver(pool, logger)
	metadataHandler := projection.HandlerFunc(func(ctx context.Context, event projection.Event) error {
		if err := resolver.Handle(ctx, event); err != nil {
			return err
		}
		return searchBuilder.HandlePageMetadataEvent(ctx, event)
	})
	consumer.Register(page.OutboxEventPageCreated, metadataHandler)
	consumer.Register(page.OutboxEventPageRenamed, metadataHandler)
	consumer.Register(page.OutboxEventRevisionPublished,
		resolver.WrapPublishedHandler(projection.NewRevisionPublishedHandler(pool, registry, logger)))
	consumer.Register(knowledge.OutboxEventClaimChanged,
		projection.NewClaimChangedHandler(pool, logger))
	consumer.Register(knowledge.OutboxEventEntityMerged,
		newEntityMergeRepairHandler(governance.NewService(
			governance.NewRepository(pool), db.NewTxManager(pool), id.NewGenerator(),
		)))
	go monitorMetrics(ctx, logger, pool, 30*time.Second)
	go metricsRegistry.MonitorDatabase(ctx, logger, pool, serviceName, cfg.ObservabilityDBInterval)
	metricsDone, err := serveMetrics(ctx, logger, cfg.WorkerMetricsAddr, metricsRegistry.Handler())
	if err != nil {
		logger.Error("启动 Worker metrics 服务失败", slog.Any("error", err))
		return exitError
	}

	var importRunner interface{ Run(context.Context) error }
	if cfg.AIImportEnabled {
		assembled, err := assembleImportRunner(ctx, pool, cfg, logger)
		if err != nil {
			logger.Error("装配 AI 导入 Worker 失败", slog.Any("error", err))
			return exitError
		}
		importRunner = assembled
		logger.Info("AI 导入消费已启用", slog.String("provider", cfg.AIProvider), slog.String("model", cfg.AIModel))
	} else {
		logger.Info("AI 导入消费未启用", slog.String("setting", "AI_IMPORT_ENABLED"))
	}

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_ = consumer.Run(ctx)
	}()
	importDone := make(chan struct{})
	if importRunner == nil {
		close(importDone)
	} else {
		go func() {
			defer close(importDone)
			_ = importRunner.Run(ctx)
		}()
	}
	linkHealthDone := make(chan struct{})
	go func() {
		defer close(linkHealthDone)
		_ = linkHealthRunner.Run(ctx, 20, time.Minute)
	}()

	<-ctx.Done()
	logger.Info("收到退出信号，等待在途事件处理完成")
	<-runDone
	<-importDone
	<-linkHealthDone
	<-metricsDone
	logger.Info("worker 已退出")
	return exitOK
}

func serveMetrics(ctx context.Context, logger *slog.Logger, addr string, handler http.Handler) (<-chan struct{}, error) {
	done := make(chan struct{})
	if addr == "" {
		close(done)
		logger.Info("Worker metrics 服务已关闭", slog.String("setting", "WORKER_METRICS_ADDR"))
		return done, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		defer close(done)
		logger.Info("Worker metrics 服务启动", slog.String("addr", listener.Addr().String()))
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				logger.Warn("关闭 Worker metrics 服务失败", slog.Any("error", err))
			}
		}()
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Worker metrics 服务异常退出", slog.Any("error", err))
		}
	}()
	return done, nil
}

// runMetrics 输出一次结构化指标快照。
func runMetrics(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) int {
	m, err := projection.CollectOperationalMetrics(ctx, pool)
	if err != nil {
		logger.Error("采集 Projection 运维指标失败", slog.Any("error", err))
		return exitError
	}
	m9, err := projection.CollectM9OperationalMetrics(ctx, pool)
	if err != nil {
		logger.Error("采集 M9 后台任务指标失败", slog.Any("error", err))
		return exitError
	}
	logMetrics(logger, m, m9)
	return exitOK
}

func logMetrics(logger *slog.Logger, m projection.OperationalMetrics, m9 projection.M9OperationalMetrics) {
	logger.Info("projection 运维指标",
		slog.Int64("outbox_backlog", m.Backlog),
		slog.Int64("outbox_pending", m.Pending),
		slog.Int64("outbox_claimed", m.Claimed),
		slog.Int64("outbox_retrying", m.Retrying),
		slog.Int64("outbox_dead", m.Dead),
		slog.Float64("outbox_oldest_backlog_seconds", m.OldestBacklogAge.Seconds()),
		slog.Int64("projection_error_states", m.ProjectionErrors),
		slog.Int64("projection_stale_states", m.StaleProjectionStates),
		slog.Int64("m9_pending_claim_changes", m9.PendingClaimChanges),
		slog.Int64("m9_component_dependencies", m9.ComponentDependencies),
		slog.Int64("m9_rule_collections", m9.RuleCollections),
		slog.Int64("m9_collection_memberships", m9.CollectionMemberships),
		slog.Int64("m9_due_external_links", m9.DueExternalLinks),
		slog.Float64("m9_oldest_external_link_due_seconds", m9.OldestExternalLinkDueAge.Seconds()),
		slog.Int64("m9_applied_entity_merges", m9.AppliedEntityMerges),
	)
}

// monitorMetrics 常驻 Worker 每 30 秒输出一次同口径指标；首次启动立即采集。
func monitorMetrics(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, interval time.Duration) {
	collect := func() {
		m, err := projection.CollectOperationalMetrics(ctx, pool)
		if err != nil {
			if ctx.Err() == nil {
				logger.Warn("采集 Projection 运维指标失败", slog.Any("error", err))
			}
			return
		}
		m9, err := projection.CollectM9OperationalMetrics(ctx, pool)
		if err != nil {
			if ctx.Err() == nil {
				logger.Warn("采集 M9 后台任务指标失败", slog.Any("error", err))
			}
			return
		}
		logMetrics(logger, m, m9)
	}
	collect()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collect()
		}
	}
}

// runConsistencyCheck 输出抽检汇总和问题定位；发现漂移以退出码 1 阻断。
func runConsistencyCheck(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, registry *projection.Registry, sampleSize int) int {
	report, err := projection.CheckConsistency(ctx, pool, registry, sampleSize)
	if err != nil {
		logger.Error("Projection 一致性抽检失败", slog.Any("error", err))
		return exitError
	}
	logger.Info("Projection 一致性抽检完成",
		slog.Int("sampled_pages", report.SampledPages),
		slog.Int("expected_states", report.ExpectedStates),
		slog.Int("consistent_states", report.ConsistentStates),
		slog.Int("issues", len(report.Issues)),
	)
	for _, issue := range report.Issues {
		attrs := []any{
			slog.String("page_id", issue.PageID.String()),
			slog.String("projection_type", issue.ProjectionType),
			slog.String("kind", string(issue.Kind)),
			slog.String("current_revision_id", issue.CurrentRevisionID.String()),
		}
		if issue.SourceRevisionID != nil {
			attrs = append(attrs, slog.String("source_revision_id", issue.SourceRevisionID.String()))
		}
		logger.Warn("Projection 一致性问题", attrs...)
	}
	if !report.Healthy() {
		return exitError
	}
	return exitOK
}

// registerBuilders 向注册表注册全部投影 Builder。
// M3-T03：page_links（页面引用投影）与 document_outline（文档大纲 + 章节锚点）；
// M3-T05：rendered_page（RenderedPage 渲染投影）；
// M3-T06：external_links（external_resource + external_link_usage）；
// M4-T07：entity_mentions / claim_usage / citation_usage；
// M7-T01/M7-T06：search（PostgreSQL staging + configurable query/sync adapter）。
func registerBuilders(reg *projection.Registry, pool *pgxpool.Pool, logger *slog.Logger, adapters ...wikisearch.SearchAdapter) *projection.SearchBuilder {
	reg.Register(projection.NewPageLinksBuilder(pool))
	reg.Register(projection.NewOutlineBuilder(pool))
	reg.Register(projection.NewExternalLinksBuilder(pool, logger))
	reg.Register(projection.NewEntityMentionsBuilder(pool))
	reg.Register(projection.NewComponentDependencyBuilder(pool))
	reg.Register(projection.NewClaimUsageBuilder(pool))
	reg.Register(projection.NewCitationUsageBuilder(pool))
	reg.Register(projection.NewRenderedPageBuilder(pool))
	searchBackend := wikisearch.SearchAdapter(wikisearch.NewPostgresAdapter(pool))
	if len(adapters) > 0 && adapters[0] != nil {
		searchBackend = adapters[0]
	}
	searchBuilder := projection.NewSearchBuilder(pool, searchBackend)
	reg.Register(searchBuilder)
	return searchBuilder
}

func configuredWorkerSearchBackend(ctx context.Context, pool *pgxpool.Pool, cfg config.Config) (wikisearch.SearchAdapter, error) {
	adapter, err := wikisearch.NewBackend(pool, wikisearch.BackendConfig{
		Backend: cfg.SearchBackend, MeiliURL: cfg.MeiliURL, MeiliAPIKey: cfg.MeiliAPIKey,
		MeiliIndex: cfg.MeiliIndex, MeiliTimeout: cfg.MeiliTimeout,
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

// runRebuild 执行一次性重建命令，返回退出码。
func runRebuild(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, registry *projection.Registry, pageArg string, all bool) int {
	rebuilder := projection.NewRebuilder(pool, registry, logger)

	if all {
		report, err := rebuilder.RebuildAll(ctx)
		if err != nil {
			logger.Error("全量投影重建失败", slog.Any("error", err))
			return exitError
		}
		logger.Info("全量投影重建完成",
			slog.Int("total", report.Total),
			slog.Int("rebuilt", report.Rebuilt),
			slog.Int("skipped", report.Skipped),
			slog.Int("failed", report.Failed),
		)
		for _, f := range report.Failures {
			logger.Warn("页面重建失败", slog.String("page_id", f.PageID.String()), slog.Any("error", f.Err))
		}
		if report.Failed > 0 {
			return exitError
		}
		return exitOK
	}

	pageID, err := uuid.Parse(pageArg)
	if err != nil {
		logger.Error("rebuild-page 参数非法，期望 Page UUID", slog.String("value", pageArg))
		return exitUsage
	}
	rebuilt, err := rebuilder.RebuildPage(ctx, pageID)
	if err != nil {
		logger.Error("页面投影重建失败", slog.String("page_id", pageID.String()), slog.Any("error", err))
		return exitError
	}
	logger.Info("页面投影重建完成",
		slog.String("page_id", pageID.String()),
		slog.Bool("rebuilt", rebuilt),
	)
	return exitOK
}
