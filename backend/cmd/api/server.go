package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/platform/observability"
)

// CheckFunc 就绪检查函数，返回 nil 表示依赖可达。
type CheckFunc func(ctx context.Context) error

// Deps /readyz 的依赖检查集合，key 为依赖名（postgres、redis）。
type Deps struct {
	Service        string
	Version        string
	Checks         map[string]CheckFunc
	Environment    string
	Authenticator  *authdomain.Authenticator
	SessionCookie  string
	TrustedOrigins []string
	Metrics        *observability.Metrics
}

// NewRouter 装配路由与中间件，handler 函数化以便单元测试。
// writeAPI/readAPI/historyAPI/projectionAPI/searchAPI 为 nil 时（未配置数据库）只暴露探针端点。
// optionalAPIs 使用类型化可选参数，兼容只装配 Page API 的测试与工具调用方。
func NewRouter(logger *slog.Logger, deps Deps, writeAPI *WriteAPI, readAPI *ReadAPI, historyAPI *HistoryAPI, projectionAPI *ProjectionAPI, searchAPI *SearchAPI, optionalAPIs ...any) http.Handler {
	var knowledgeReadAPI *KnowledgeReadAPI
	var governanceAPI *GovernanceAPI
	var importAPI *ImportAPI
	var authAPI *AuthAPI
	var collaborationAPI *CollaborationAPI
	var collectionAPI *CollectionAPI
	for _, optional := range optionalAPIs {
		switch api := optional.(type) {
		case *KnowledgeReadAPI:
			knowledgeReadAPI = api
		case *GovernanceAPI:
			governanceAPI = api
		case *ImportAPI:
			importAPI = api
		case *AuthAPI:
			authAPI = api
		case *CollaborationAPI:
			collaborationAPI = api
		case *CollectionAPI:
			collectionAPI = api
		}
	}
	r := chi.NewRouter()
	r.Use(RequestID)
	if deps.Metrics != nil {
		r.Use(deps.Metrics.HTTPMiddleware(deps.Service))
	}
	r.Use(Recoverer(logger, deps.Service, deps.Metrics))
	r.Use(AccessLog(logger))
	r.Use(Authentication(deps.Authenticator, deps.Environment == ""))
	r.Use(BrowserWriteGuard(deps.SessionCookie, deps.TrustedOrigins))

	r.Get("/healthz", healthzHandler(deps.Service, deps.Version))
	r.Get("/readyz", readyzHandler(deps.Checks))
	if deps.Metrics != nil {
		r.Handle("/metrics", deps.Metrics.Handler())
	}

	if writeAPI != nil || readAPI != nil || historyAPI != nil || projectionAPI != nil || searchAPI != nil || knowledgeReadAPI != nil || governanceAPI != nil || importAPI != nil || authAPI != nil || collaborationAPI != nil || collectionAPI != nil {
		r.Route("/api/v1", func(r chi.Router) {
			if authAPI != nil {
				r.Get("/auth/login", authAPI.login)
				r.Get("/auth/callback", authAPI.callback)
				r.Get("/auth/session", authAPI.session)
				r.Post("/auth/logout", authAPI.logout)
			}
			if writeAPI != nil {
				r.Post("/pages", writeAPI.createPage)
				r.Post("/pages/{id}/rename", writeAPI.renamePage)
				r.Post("/pages/{id}/revisions", writeAPI.publishRevision)
			}
			if collaborationAPI != nil {
				r.Get("/pages/{id}/collaboration", collaborationAPI.connect)
			}
			if readAPI != nil {
				r.Get("/pages/by-title", readAPI.getPageByTitle)
				r.Get("/pages/{id}", readAPI.getPageByID)
			}
			if searchAPI != nil {
				r.Get("/pages/search", searchAPI.searchPages)
			}
			if historyAPI != nil {
				r.Get("/pages/{id}/revisions", historyAPI.listRevisions)
				r.Get("/pages/{id}/revisions/{rid}", historyAPI.getRevision)
				r.Get("/pages/{id}/diff", historyAPI.diffRevisions)
				r.Post("/pages/{id}/rollback", historyAPI.rollback)
			}
			if projectionAPI != nil {
				r.Get("/pages/{id}/backlinks", projectionAPI.listBacklinks)
				r.Get("/pages/{id}/outline", projectionAPI.getOutline)
				r.Get("/pages/{id}/anchors/{slug}", projectionAPI.resolveAnchor)
				r.Get("/entities/{id}/mentions", projectionAPI.listEntityMentions)
				r.Get("/claims/{id}/usages", projectionAPI.listClaimUsages)
				r.Get("/citations/{id}/usages", projectionAPI.listCitationUsages)
			}
			if knowledgeReadAPI != nil {
				r.Get("/entities/{id}", knowledgeReadAPI.getEntity)
				r.Post("/entities/{id}/merge", knowledgeReadAPI.mergeEntity)
				r.Get("/claims/{id}", knowledgeReadAPI.getClaim)
				r.Get("/citations/{id}", knowledgeReadAPI.getCitation)
			}
			if collectionAPI != nil {
				r.Get("/collections", collectionAPI.list)
				r.Get("/collections/{id}", collectionAPI.get)
				r.Get("/collections/{id}/members", collectionAPI.members)
			}
			if governanceAPI != nil {
				r.Post("/proposals", governanceAPI.createProposal)
				r.Get("/proposals/{id}", governanceAPI.getProposal)
				r.Post("/proposals/{id}/operations", governanceAPI.addOperation)
				r.Post("/proposals/{id}/submit", governanceAPI.submitProposal)
				r.Get("/proposals/{id}/preview", governanceAPI.previewProposal)
				r.Post("/proposals/{id}/apply", governanceAPI.applyProposal)
				r.Post("/proposals/{id}/merge-to-working-document", governanceAPI.mergeToWorkingDocument)
				r.Post("/proposals/{id}/conflicts/{conflict_id}/resolution", governanceAPI.resolveConflict)
				r.Get("/review-tasks", governanceAPI.pendingReviews)
				r.Post("/review-tasks/{id}/decision", governanceAPI.decideReview)
				r.Post("/change-batches/{id}/rollback", governanceAPI.rollbackBatch)
				r.Post("/bulk-review-batches", governanceAPI.createBulkReview)
				r.Get("/bulk-review-batches/{id}", governanceAPI.getBulkReview)
				r.Post("/bulk-review-batches/{id}/proposals/{proposal_id}/decision", governanceAPI.decideBulkReview)
				r.Post("/bulk-review-batches/{id}/finalize", governanceAPI.finalizeBulkReview)
				r.Post("/bulk-review-batches/{id}/pause", governanceAPI.pauseBulkReview)
				r.Post("/bulk-review-batches/{id}/resume", governanceAPI.resumeBulkReview)
				r.Post("/bulk-review-batches/{id}/apply-next-wave", governanceAPI.applyBulkReviewWave)
				r.Get("/bulk-review-batches/{id}/audit-events", governanceAPI.bulkReviewAudit)
			}
			if importAPI != nil {
				r.Post("/import-jobs", importAPI.createJob)
				r.Post("/import-jobs/uploads", importAPI.createUploadJob)
				r.Get("/import-jobs/{id}", importAPI.getJob)
				r.Post("/import-jobs/{id}/cancel", importAPI.cancelJob)
				r.Post("/import-jobs/{id}/retry", importAPI.retryJob)
			}
		})
	}
	return r
}

// healthzHandler 存活探针：恒 200，返回 service/version。
func healthzHandler(service, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{
			"service": service,
			"version": version,
		})
	}
}

// readyzHandler 就绪探针：逐项检查依赖可达性；
// 依赖未配置时报告 not_configured 而非崩溃。
func readyzHandler(checks map[string]CheckFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results := make(map[string]string, len(checks))
		ready := true
		for name, check := range checks {
			if check == nil {
				results[name] = "not_configured"
				ready = false
				continue
			}
			if err := check(r.Context()); err != nil {
				results[name] = "error: " + err.Error()
				ready = false
				continue
			}
			results[name] = "ok"
		}
		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}
		httpx.WriteJSON(w, status, map[string]any{
			"status": map[bool]string{true: "ok", false: "unavailable"}[ready],
			"checks": results,
		})
	}
}
