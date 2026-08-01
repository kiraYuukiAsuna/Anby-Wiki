package main

import (
	"net/http"
	"time"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type revisionStorageStatsResponse struct {
	ArchiveAvailable   bool       `json:"archive_available"`
	RetentionSeconds   int64      `json:"retention_seconds"`
	DefaultBatchSize   int        `json:"default_batch_size"`
	HotSnapshots       int64      `json:"hot_snapshots"`
	ColdSnapshots      int64      `json:"cold_snapshots"`
	HotBytes           int64      `json:"hot_bytes"`
	ColdBytes          int64      `json:"cold_bytes"`
	EligibleSnapshots  int64      `json:"eligible_snapshots"`
	OldestHotCreatedAt *time.Time `json:"oldest_hot_created_at"`
}

type archiveRevisionSnapshotsRequest struct {
	Limit int `json:"limit"`
}

type snapshotArchiveResultResponse struct {
	Examined      int   `json:"examined"`
	Archived      int   `json:"archived"`
	Skipped       int   `json:"skipped"`
	ArchivedBytes int64 `json:"archived_bytes"`
}

func (a *GovernanceAPI) revisionStorageStats(
	w http.ResponseWriter,
	r *http.Request,
) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	if a.auth == nil {
		governanceError(w, r, governance.ErrPermissionDenied)
		return
	}
	if err := a.auth.Check(
		r.Context(), actorID, a.wikiID, governance.ActionManage, nil,
	); err != nil {
		governanceError(w, r, err)
		return
	}
	stats, err := a.revisions.SnapshotStorageStats(r.Context())
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toRevisionStorageStats(stats))
}

func (a *GovernanceAPI) archiveRevisionSnapshots(
	w http.ResponseWriter,
	r *http.Request,
) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	if a.auth == nil {
		governanceError(w, r, governance.ErrPermissionDenied)
		return
	}
	if err := a.auth.Check(
		r.Context(), actorID, a.wikiID, governance.ActionManage, nil,
	); err != nil {
		governanceError(w, r, err)
		return
	}
	var req archiveRevisionSnapshotsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Limit < 0 || req.Limit > 500 {
		httpx.WriteError(
			w, r, http.StatusBadRequest, httpx.CodeValidationFailed,
			"limit 必须是 1..500；省略或传 0 使用服务端默认值",
		)
		return
	}
	result, err := a.revisions.ArchiveRevisionSnapshots(
		r.Context(), req.Limit,
	)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, snapshotArchiveResultResponse{
		Examined: result.Examined, Archived: result.Archived,
		Skipped: result.Skipped, ArchivedBytes: result.ArchivedBytes,
	})
}

func toRevisionStorageStats(
	stats *page.RevisionStorageStats,
) revisionStorageStatsResponse {
	return revisionStorageStatsResponse{
		ArchiveAvailable: stats.ArchiveAvailable,
		RetentionSeconds: int64(stats.Retention / time.Second),
		DefaultBatchSize: stats.DefaultBatchSize,
		HotSnapshots:     stats.HotSnapshots, ColdSnapshots: stats.ColdSnapshots,
		HotBytes: stats.HotBytes, ColdBytes: stats.ColdBytes,
		EligibleSnapshots:  stats.EligibleSnapshots,
		OldestHotCreatedAt: stats.OldestHotCreatedAt,
	}
}
