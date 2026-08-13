package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/platform/observability"
	"github.com/anby/wiki/backend/internal/platform/ratelimit"
)

// CheckFunc 就绪检查函数，返回 nil 表示依赖可达。
type CheckFunc func(ctx context.Context) error

// Deps /readyz 的依赖检查集合，key 为依赖名（postgres、redis）。
type Deps struct {
	Service       string
	Version       string
	Checks        map[string]CheckFunc
	Environment   string
	Authenticator *authdomain.Authenticator
	// AllowDevActorHeader preserves X-Actor-ID only for explicit local
	// development/test mode. Production always strips it before auth.
	AllowDevActorHeader bool
	Metrics             *observability.Metrics
	// RateLimiter is nil when rate limiting is disabled or Redis is
	// unavailable; the middleware is then skipped entirely.
	RateLimiter     *ratelimit.Limiter
	RateLimitConfig RateLimitConfig
}

// NewRouter 装配路由与中间件。
// writeAPI/readAPI/historyAPI/projectionAPI/searchAPI 为 nil 时（未配置数据库）只暴露探针端点。
// optionalAPIs 使用类型化可选参数，允许按启动能力增量装配。
func NewRouter(logger *slog.Logger, deps Deps, writeAPI *WriteAPI, readAPI *ReadAPI, historyAPI *HistoryAPI, projectionAPI *ProjectionAPI, searchAPI *SearchAPI, optionalAPIs ...any) http.Handler {
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
		case *AssetAPI:
			assetAPI = api
		case *DatasetAPI:
			datasetAPI = api
		case *ComponentAPI:
			componentAPI = api
		case *SourceAPI:
			sourceAPI = api
		case *AIConfigAPI:
			aiConfigAPI = api
		}
	}
	r := chi.NewRouter()
	r.Use(RequestID)
	r.Use(SecurityHeaders)
	r.Use(StripSpoofableAuthHeaders(deps.AllowDevActorHeader))
	r.Use(RequestBodyLimit)
	if deps.RateLimiter != nil {
		r.Use(RateLimit(deps.RateLimiter, deps.RateLimitConfig, logger))
	}
	if deps.Metrics != nil {
		r.Use(deps.Metrics.HTTPMiddleware(deps.Service))
	}
	r.Use(Recoverer(logger, deps.Service, deps.Metrics))
	r.Use(AccessLog(logger))
	r.Use(Authentication(deps.Authenticator, deps.Environment == ""))

	r.Get("/healthz", healthzHandler(deps.Service, deps.Version))
	r.Get("/readyz", readyzHandler(deps.Checks))
	if deps.Metrics != nil {
		r.Handle("/metrics", deps.Metrics.Handler())
	}

	if writeAPI != nil || readAPI != nil || historyAPI != nil || projectionAPI != nil || searchAPI != nil || knowledgeReadAPI != nil || governanceAPI != nil || importAPI != nil || authAPI != nil || collaborationAPI != nil || collectionAPI != nil || assetAPI != nil || datasetAPI != nil || componentAPI != nil || sourceAPI != nil || aiConfigAPI != nil {
		r.Route("/api/v1", func(r chi.Router) {
			if authAPI != nil {
				r.Post("/auth/register", authAPI.register)
				r.Post("/auth/login", authAPI.login)
				r.Get("/auth/session", authAPI.session)
				r.Post("/auth/logout", authAPI.logout)
			}
			if writeAPI != nil {
				r.Post("/pages", writeAPI.createPage)
				r.Post("/pages/{id}/rename", writeAPI.renamePage)
				r.Get("/pages/{id}/redirect", writeAPI.getRedirect)
				r.Post("/pages/{id}/redirect", writeAPI.createRedirect)
				r.Delete("/pages/{id}/redirect", writeAPI.deleteRedirect)
				r.Get("/pages/{id}/block-redirects", writeAPI.listBlockRedirects)
				r.Put("/pages/{id}/block-redirects/{block_id}", writeAPI.upsertBlockRedirect)
				r.Delete("/pages/{id}/block-redirects/{block_id}", writeAPI.deleteBlockRedirect)
				r.Post("/pages/{id}/revisions", writeAPI.publishRevision)
			}
			if collaborationAPI != nil {
				r.Get("/pages/{id}/collaboration", collaborationAPI.connect)
			}
			if readAPI != nil {
				r.Get("/pages", readAPI.listPages)
				r.Get("/pages/by-title", readAPI.getPageByTitle)
				r.Get("/pages/{id}", readAPI.getPageByID)
			}
			if searchAPI != nil {
				r.Get("/pages/search", searchAPI.searchPages)
				r.Get("/search/capabilities", searchAPI.capabilities)
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
				r.Get("/pages/{id}/references", projectionAPI.getReferences)
				r.Get("/pages/{id}/related", projectionAPI.getRelatedPages)
				r.Get("/pages/{id}/sections", projectionAPI.getSections)
				r.Get("/pages/{id}/sections/{section_key}", projectionAPI.getSection)
				r.Get("/pages/{id}/section-locator/{block_id}", projectionAPI.locateSection)
				r.Get("/pages/{id}/anchors/{slug}", projectionAPI.resolveAnchor)
				r.Get("/entities/{id}/mentions", projectionAPI.listEntityMentions)
				r.Get("/claims/{id}/usages", projectionAPI.listClaimUsages)
				r.Get("/citations/{id}/usages", projectionAPI.listCitationUsages)
				r.Get("/sources/{id}/usages", projectionAPI.listSourceUsages)
				r.Get("/sources/{id}/usages/{page_id}", projectionAPI.listSourceUsageLocations)
				r.Get("/components/{id}/usages", projectionAPI.listComponentUsages)
			}
			if knowledgeReadAPI != nil {
				r.Get("/entities", knowledgeReadAPI.listEntities)
				r.Get("/entities/{id}", knowledgeReadAPI.getEntity)
				r.Post("/entities/{id}/labels", knowledgeReadAPI.addEntityLabel)
				r.Delete("/entities/{id}/labels", knowledgeReadAPI.removeEntityLabel)
				r.Put("/entities/{id}/labels/primary", knowledgeReadAPI.setPrimaryEntityLabel)
				r.Post("/entities/{id}/aliases", knowledgeReadAPI.addEntityAlias)
				r.Delete("/entities/{id}/aliases/{alias_id}", knowledgeReadAPI.removeEntityAlias)
				r.Get("/entities/{id}/graph", knowledgeReadAPI.getEntityGraph)
				r.Get("/entities/{id}/merge", knowledgeReadAPI.getEntityMerge)
				r.Post("/entities/{id}/merge", knowledgeReadAPI.mergeEntity)
				r.Post("/entity-merges/{id}/rollback", knowledgeReadAPI.rollbackEntityMerge)
				r.Get("/entities/{id}/federation-links", knowledgeReadAPI.listEntityFederationLinks)
				r.Post("/entities/{id}/federation-links", knowledgeReadAPI.createFederationLink)
				r.Post("/entity-graph/rebuild", knowledgeReadAPI.rebuildEntityGraph)
				r.Get("/federated-wikis", knowledgeReadAPI.listFederatedWikis)
				r.Post("/federated-wikis", knowledgeReadAPI.registerFederatedWiki)
				r.Put("/federated-wikis/{id}", knowledgeReadAPI.updateFederatedWiki)
				r.Get("/federation-links", knowledgeReadAPI.listFederationLinks)
				r.Put("/federation-links/{id}", knowledgeReadAPI.updateFederationLink)
				r.Get("/claims/{id}", knowledgeReadAPI.getClaim)
				r.Put("/claims/{id}/verification", knowledgeReadAPI.updateClaimVerification)
				r.Get("/citations/{id}", knowledgeReadAPI.getCitation)
				r.Get("/pages/{id}/entity-bindings", knowledgeReadAPI.listPageEntityBindings)
				r.Put("/pages/{id}/entity-bindings", knowledgeReadAPI.writePageEntityBinding)
				r.Delete(
					"/pages/{id}/entity-bindings/{entity_id}",
					knowledgeReadAPI.removePageEntityBinding,
				)
				if knowledgeReadAPI.consistency != nil {
					r.Get("/fact-consistency-issues", knowledgeReadAPI.listFactConsistencyIssues)
					r.Post("/fact-consistency-scans", knowledgeReadAPI.scanFactConsistency)
				}
			}
			if collectionAPI != nil {
				r.Get("/pages/{id}/collections", collectionAPI.listForPage)
				r.Get("/collections", collectionAPI.list)
				r.Post("/collections", collectionAPI.create)
				r.Get("/collections/{id}", collectionAPI.get)
				r.Get("/collections/{id}/members", collectionAPI.members)
				r.Put("/collections/{id}/members", collectionAPI.replaceMembers)
				r.Post("/collections/{id}/rebuild", collectionAPI.rebuild)
			}
			if assetAPI != nil {
				r.Get("/assets", assetAPI.list)
				r.Post("/assets", assetAPI.upload)
				r.Get("/assets/revisions/{revision_id}", assetAPI.getRevision)
				r.Get("/assets/revisions/{revision_id}/content", assetAPI.content)
			}
			if datasetAPI != nil {
				r.Get("/datasets", datasetAPI.list)
				r.Post("/datasets", datasetAPI.create)
				r.Get("/datasets/{id}", datasetAPI.get)
				r.Get("/datasets/{id}/records", datasetAPI.listRecords)
				r.Post("/datasets/{id}/records", datasetAPI.createRecord)
				r.Get("/datasets/{id}/views", datasetAPI.listViews)
				r.Post("/datasets/{id}/views", datasetAPI.createView)
				r.Put("/dataset-records/{id}", datasetAPI.updateRecord)
				r.Get("/dataset-views/{id}", datasetAPI.getView)
				r.Get("/dataset-views/{id}/records", datasetAPI.queryView)
			}
			if componentAPI != nil {
				r.Get("/components", componentAPI.list)
				r.Post("/components", componentAPI.create)
				r.Get("/components/{id}", componentAPI.get)
				r.Get("/components/{id}/versions", componentAPI.listVersions)
				r.Post("/components/{id}/versions", componentAPI.createVersion)
				r.Put("/components/{id}/versions/{version}", componentAPI.updateVersion)
				r.Post("/components/{id}/versions/{version}/publish", componentAPI.publishVersion)
				r.Post("/components/{id}/versions/{version}/deprecate", componentAPI.deprecateVersion)
				r.Post("/components/{id}/versions/{version}/preview", componentAPI.preview)
			}
			if sourceAPI != nil {
				r.Get("/sources", sourceAPI.list)
				r.Post("/sources", sourceAPI.create)
				r.Get("/sources/{id}", sourceAPI.get)
				r.Get("/sources/{id}/versions", sourceAPI.versions)
				r.Get("/source-versions/{id}/chunks", sourceAPI.chunks)
				r.Post("/citations", sourceAPI.createCitation)
			}
			if governanceAPI != nil {
				r.Get("/proposals", governanceAPI.listProposals)
				r.Get("/apply-queue", governanceAPI.listPendingApplyProposals)
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
				r.Get("/bulk-review-batches", governanceAPI.listBulkReviews)
				r.Post("/bulk-review-batches", governanceAPI.createBulkReview)
				r.Get("/bulk-review-batches/{id}", governanceAPI.getBulkReview)
				r.Post("/bulk-review-batches/{id}/proposals/{proposal_id}/decision", governanceAPI.decideBulkReview)
				r.Post("/bulk-review-batches/{id}/finalize", governanceAPI.finalizeBulkReview)
				r.Post("/bulk-review-batches/{id}/pause", governanceAPI.pauseBulkReview)
				r.Post("/bulk-review-batches/{id}/resume", governanceAPI.resumeBulkReview)
				r.Post("/bulk-review-batches/{id}/apply-next-wave", governanceAPI.applyBulkReviewWave)
				r.Get("/bulk-review-batches/{id}/audit-events", governanceAPI.bulkReviewAudit)
				if governanceAPI.audit != nil {
					r.Get("/change-tags", governanceAPI.listChangeTags)
					r.Post("/change-tags", governanceAPI.createChangeTag)
					r.Post("/change-tags/{id}/assignments", governanceAPI.assignChangeTag)
					r.Get("/audit-events", governanceAPI.listAuditEvents)
				}
				if governanceAPI.protection != nil {
					r.Get("/roles", governanceAPI.listRoles)
					r.Get("/page-protections", governanceAPI.listPageProtections)
					r.Post("/page-protections", governanceAPI.createPageProtection)
					r.Delete("/page-protections/{id}", governanceAPI.deletePageProtection)
				}
				if governanceAPI.aiTrust != nil {
					r.Get("/ai-trust-profiles", governanceAPI.listAITrustProfiles)
					r.Put("/ai-trust-profiles/{actor_id}", governanceAPI.updateAITrustProfile)
				}
				if governanceAPI.revisions != nil {
					r.Get("/revision-storage", governanceAPI.revisionStorageStats)
					r.Post(
						"/revision-storage/archive",
						governanceAPI.archiveRevisionSnapshots,
					)
				}
			}
			if importAPI != nil {
				r.Get("/import-jobs", importAPI.listJobs)
				r.Post("/import-jobs", importAPI.createJob)
				r.Post("/import-jobs/uploads", importAPI.createUploadJob)
				r.Get("/import-jobs/{id}", importAPI.getJob)
				r.Post("/import-jobs/{id}/cancel", importAPI.cancelJob)
				r.Post("/import-jobs/{id}/retry", importAPI.retryJob)
			}
			if aiConfigAPI != nil {
				r.Get("/admin/ai-config", aiConfigAPI.get)
				r.Put("/admin/ai-config", aiConfigAPI.update)
				r.Post("/admin/ai-config/test", aiConfigAPI.test)
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
