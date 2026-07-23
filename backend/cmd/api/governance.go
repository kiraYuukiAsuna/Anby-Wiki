package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/collaboration"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type GovernanceAPI struct {
	proposals  *governance.Service
	repo       *governance.Repository
	preview    *governance.PreviewService
	reviews    *governance.ReviewService
	apply      *governance.ApplyService
	rollback   *governance.RollbackService
	merge      *governance.MergeWorkingDocumentService
	hub        *collaboration.Hub
	resolution *governance.ConflictResolutionService
	bulkReview *governance.BulkReviewService
}

func (a *GovernanceAPI) WithBulkReview(service *governance.BulkReviewService) *GovernanceAPI {
	a.bulkReview = service
	return a
}

func (a *GovernanceAPI) WithConflictResolution(
	service *governance.ConflictResolutionService,
) *GovernanceAPI {
	a.resolution = service
	return a
}

func (a *GovernanceAPI) WithWorkingDocumentMerge(
	service *governance.MergeWorkingDocumentService,
	hub *collaboration.Hub,
) *GovernanceAPI {
	a.merge = service
	a.hub = hub
	return a
}

func NewGovernanceAPI(proposals *governance.Service, repo *governance.Repository,
	preview *governance.PreviewService, reviews *governance.ReviewService,
	apply *governance.ApplyService, rollback *governance.RollbackService) *GovernanceAPI {
	return &GovernanceAPI{proposals: proposals, repo: repo, preview: preview,
		reviews: reviews, apply: apply, rollback: rollback}
}

type createProposalRequest struct {
	ImportJobID      *uuid.UUID `json:"import_job_id"`
	TargetType       string     `json:"target_type"`
	TargetID         *uuid.UUID `json:"target_id"`
	BaseRevisionID   *uuid.UUID `json:"base_revision_id"`
	BaseStateVersion *int       `json:"base_state_version"`
	RiskLevel        string     `json:"risk_level"`
}

type proposalResponse struct {
	ID               uuid.UUID                  `json:"id"`
	ImportJobID      *uuid.UUID                 `json:"import_job_id"`
	TargetType       string                     `json:"target_type"`
	TargetID         *uuid.UUID                 `json:"target_id"`
	BaseRevisionID   *uuid.UUID                 `json:"base_revision_id"`
	BaseStateVersion *int                       `json:"base_state_version"`
	Status           string                     `json:"status"`
	RiskLevel        string                     `json:"risk_level"`
	RiskReasons      json.RawMessage            `json:"risk_reasons"`
	PolicyDecision   json.RawMessage            `json:"policy_decision"`
	CreatedBy        uuid.UUID                  `json:"created_by"`
	IdempotencyKey   string                     `json:"idempotency_key"`
	CreatedAt        string                     `json:"created_at"`
	UpdatedAt        string                     `json:"updated_at"`
	Operations       []operationRecordResponse  `json:"operations"`
	Conflicts        []governance.MergeConflict `json:"conflicts"`
}

type operationRecordResponse struct {
	ID        uuid.UUID              `json:"id"`
	Sequence  int                    `json:"sequence"`
	Operation governance.OperationV1 `json:"operation"`
}

func (a *GovernanceAPI) createProposal(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "缺少 Idempotency-Key 请求头")
		return
	}
	var req createProposalRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	p, err := a.proposals.CreateProposal(r.Context(), governance.CreateProposalParams{
		ImportJobID: req.ImportJobID, TargetType: req.TargetType, TargetID: req.TargetID,
		BaseRevisionID: req.BaseRevisionID, BaseStateVersion: req.BaseStateVersion,
		RiskLevel: req.RiskLevel, CreatedBy: actorID, IdempotencyKey: key,
	})
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toProposalResponse(p, nil, nil))
}

func (a *GovernanceAPI) getProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	p, err := a.repo.GetProposal(r.Context(), nil, id)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	records, err := a.repo.ListOperations(r.Context(), nil, id)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	operations := make([]operationRecordResponse, len(records))
	for i := range records {
		op, err := governance.OperationFromRecord(&records[i])
		if err != nil {
			governanceError(w, r, err)
			return
		}
		operations[i] = operationRecordResponse{ID: records[i].ID, Sequence: records[i].Sequence, Operation: *op}
	}
	conflicts, err := a.repo.ListMergeConflicts(r.Context(), id)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toProposalResponse(p, operations, conflicts))
}

func (a *GovernanceAPI) addOperation(w http.ResponseWriter, r *http.Request) {
	id, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "Operation 请求体过大或不可读")
		return
	}
	record, err := a.proposals.AddOperationV1(r.Context(), id, raw)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	op, _ := governance.OperationFromRecord(record)
	httpx.WriteJSON(w, http.StatusCreated, operationRecordResponse{ID: record.ID, Sequence: record.Sequence, Operation: *op})
}

func (a *GovernanceAPI) submitProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	result, err := a.reviews.Submit(r.Context(), id)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *GovernanceAPI) previewProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	result, err := a.preview.PreviewPageProposal(r.Context(), id)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *GovernanceAPI) pendingReviews(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	tasks, err := a.reviews.Pending(r.Context(), limit)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": tasks})
}

type reviewDecisionRequest struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}

func (a *GovernanceAPI) decideReview(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	id, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	var req reviewDecisionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	p, err := a.reviews.Decide(r.Context(), id, actorID, req.Approve, req.Reason)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toProposalResponse(p, nil, nil))
}

func (a *GovernanceAPI) applyProposal(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	id, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	result, err := a.apply.Apply(r.Context(), id, actorID)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

type mergeWorkingDocumentRequest struct {
	WorkingDocumentID uuid.UUID       `json:"working_document_id"`
	ExpectedSequence  int64           `json:"expected_sequence"`
	ClientID          uuid.UUID       `json:"client_id"`
	ClientUpdateID    uuid.UUID       `json:"client_update_id"`
	CurrentAST        json.RawMessage `json:"current_ast"`
	MergedAST         json.RawMessage `json:"merged_ast"`
	UpdateBase64      string          `json:"update_base64"`
}

func (a *GovernanceAPI) mergeToWorkingDocument(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	proposalID, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	if a.merge == nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "工作副本合并服务未配置")
		return
	}
	var req mergeWorkingDocumentRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	update, err := base64.StdEncoding.DecodeString(req.UpdateBase64)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "update_base64 非法")
		return
	}
	result, accepted, err := a.merge.Merge(r.Context(), governance.MergeWorkingDocumentParams{
		ProposalID: proposalID, DocumentID: req.WorkingDocumentID, ActorID: actorID,
		ClientID: req.ClientID, ClientUpdateID: req.ClientUpdateID,
		ExpectedSequence: req.ExpectedSequence, CurrentAST: req.CurrentAST,
		MergedAST: req.MergedAST, Update: update,
	})
	if err != nil {
		governanceError(w, r, err)
		return
	}
	if accepted != nil {
		frame, err := collaboration.EncodeServerFrame(
			collaboration.FrameUpdate, accepted.Sequence, accepted.Bytes,
		)
		if err == nil && a.hub != nil {
			a.hub.Broadcast(
				accepted.DocumentID,
				collaboration.HubMessage{Binary: true, Data: frame},
			)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

type resolveConflictRequest struct {
	Choice string `json:"choice"`
	Reason string `json:"reason"`
}

func (a *GovernanceAPI) resolveConflict(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	proposalID, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	conflictID, ok := governancePathID(w, r, "conflict_id")
	if !ok {
		return
	}
	if a.resolution == nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "冲突解决服务未配置")
		return
	}
	var req resolveConflictRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	proposal, err := a.resolution.Resolve(r.Context(), governance.ResolveConflictParams{
		ProposalID: proposalID, ConflictID: conflictID, ActorID: actorID,
		Choice: req.Choice, Reason: req.Reason,
	})
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toProposalResponse(proposal, nil, nil))
}

func (a *GovernanceAPI) rollbackBatch(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	id, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	result, err := a.rollback.Rollback(r.Context(), id, actorID)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

type createBulkReviewRequest struct {
	ProposalIDs   []uuid.UUID `json:"proposal_ids"`
	SamplePercent int         `json:"sample_percent"`
	ForceFull     bool        `json:"force_full"`
	WaveSize      int         `json:"wave_size"`
}

func (a *GovernanceAPI) createBulkReview(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	var req createBulkReviewRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	batch, err := a.bulkReview.Create(r.Context(), governance.CreateBulkReviewParams{
		ProposalIDs: req.ProposalIDs, CreatedBy: actorID, SamplePercent: req.SamplePercent,
		ForceFull: req.ForceFull, WaveSize: req.WaveSize,
	})
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, batch)
}

func (a *GovernanceAPI) getBulkReview(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	id, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	batch, err := a.bulkReview.Get(r.Context(), id, actorID)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, batch)
}

type bulkReviewDecisionRequest struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}

func (a *GovernanceAPI) decideBulkReview(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	batchID, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	proposalID, ok := governancePathID(w, r, "proposal_id")
	if !ok {
		return
	}
	var req bulkReviewDecisionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	batch, err := a.bulkReview.Decide(r.Context(), batchID, proposalID, actorID, req.Approve, req.Reason)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, batch)
}

func (a *GovernanceAPI) finalizeBulkReview(w http.ResponseWriter, r *http.Request) {
	a.bulkReviewActorAction(w, r, a.bulkReview.Finalize)
}

func (a *GovernanceAPI) pauseBulkReview(w http.ResponseWriter, r *http.Request) {
	a.bulkReviewActorAction(w, r, a.bulkReview.Pause)
}

func (a *GovernanceAPI) resumeBulkReview(w http.ResponseWriter, r *http.Request) {
	a.bulkReviewActorAction(w, r, a.bulkReview.Resume)
}

func (a *GovernanceAPI) bulkReviewActorAction(w http.ResponseWriter, r *http.Request,
	action func(context.Context, uuid.UUID, uuid.UUID) (*governance.BulkReviewBatch, error)) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	batchID, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	batch, err := action(r.Context(), batchID, actorID)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, batch)
}

func (a *GovernanceAPI) applyBulkReviewWave(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	batchID, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	result, err := a.bulkReview.ApplyNextWave(r.Context(), batchID, actorID)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *GovernanceAPI) bulkReviewAudit(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	batchID, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	events, err := a.bulkReview.Audit(r.Context(), batchID, actorID)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": events})
}

func governancePathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "路径 id 不是合法 UUID")
		return uuid.Nil, false
	}
	return id, true
}

func toProposalResponse(p *governance.Proposal, operations []operationRecordResponse, conflicts []governance.MergeConflict) proposalResponse {
	if operations == nil {
		operations = []operationRecordResponse{}
	}
	if conflicts == nil {
		conflicts = []governance.MergeConflict{}
	}
	return proposalResponse{
		ID: p.ID, ImportJobID: p.ImportJobID, TargetType: p.TargetType, TargetID: p.TargetID,
		BaseRevisionID: p.BaseRevisionID, BaseStateVersion: p.BaseStateVersion,
		Status: p.Status, RiskLevel: p.RiskLevel, RiskReasons: p.RiskReasons,
		PolicyDecision: p.PolicyDecision, CreatedBy: p.CreatedBy, IdempotencyKey: p.IdempotencyKey,
		CreatedAt:  p.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		UpdatedAt:  p.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		Operations: operations, Conflicts: conflicts,
	}
}

func governanceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, governance.ErrProposalNotFound), errors.Is(err, governance.ErrBulkReviewNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, governance.ErrInvalidActor), errors.Is(err, governance.ErrActorNotAllowed), errors.Is(err, governance.ErrPermissionDenied):
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, err.Error())
	case errors.Is(err, governance.ErrInvalidProposal), errors.Is(err, governance.ErrInvalidOperation), errors.Is(err, governance.ErrProposalHasNoOps):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, err.Error())
	case errors.Is(err, governance.ErrInvalidResolution):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, err.Error())
	case errors.Is(err, governance.ErrInvalidTransition), errors.Is(err, governance.ErrProposalNotDraft),
		errors.Is(err, governance.ErrPatchTargetModified), errors.Is(err, governance.ErrMergeConflict),
		errors.Is(err, governance.ErrRollbackStale), errors.Is(err, governance.ErrApprovalRequired),
		errors.Is(err, governance.ErrBulkReviewIncomplete), errors.Is(err, governance.ErrBulkReviewPaused),
		errors.Is(err, governance.ErrMergedASTMismatch),
		errors.Is(err, collaboration.ErrSequenceMismatch),
		errors.Is(err, collaboration.ErrIdempotencyConflict),
		errors.Is(err, collaboration.ErrDocumentInactive),
		errors.Is(err, page.ErrStaleRevision):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, collaboration.ErrDocumentNotFound), errors.Is(err, collaboration.ErrDocumentPageMismatch):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	default:
		serviceError(w, r, err)
	}
}
