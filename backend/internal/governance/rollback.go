package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/collection"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var ErrRollbackStale = errors.New("governance: ChangeBatch 之后已有新变更，拒绝静默回滚")

type RollbackService struct {
	repo        *Repository
	pages       *page.Service
	knowledge   *KnowledgePatchEngine
	collections *collection.Service
	txm         *db.TxManager
	ids         *id.Generator
	auth        *AuthorizationService
}

func (s *RollbackService) WithAuthorization(auth *AuthorizationService) *RollbackService {
	s.auth = auth
	return s
}

func (s *RollbackService) WithCollections(service *collection.Service) *RollbackService {
	s.collections = service
	return s
}

func NewRollbackService(
	repo *Repository,
	pages *page.Service,
	knowledge *KnowledgePatchEngine,
	txm *db.TxManager,
	ids *id.Generator,
) *RollbackService {
	return &RollbackService{
		repo: repo, pages: pages, knowledge: knowledge, txm: txm, ids: ids,
	}
}

type RollbackResult struct {
	ChangeBatchID           uuid.UUID   `json:"change_batch_id"`
	RevisionIDs             []uuid.UUID `json:"revision_ids"`
	CompensationClaimIDs    []uuid.UUID `json:"compensation_claim_ids"`
	CompensatedPageIDs      []uuid.UUID `json:"compensated_page_ids"`
	DeletedPageIDs          []uuid.UUID `json:"deleted_page_ids"`
	DeletedEntityIDs        []uuid.UUID `json:"deleted_entity_ids"`
	RestoredEntityIDs       []uuid.UUID `json:"restored_entity_ids"`
	EntityMergeIDs          []uuid.UUID `json:"entity_merge_ids"`
	CollectionIDs           []uuid.UUID `json:"collection_ids"`
	RemovedClaimSourceCount int         `json:"removed_claim_source_count"`
	Idempotent              bool        `json:"idempotent"`
}

type rollbackRevisionSpan struct {
	PageID        uuid.UUID
	FirstParentID *uuid.UUID
	LastID        uuid.UUID
}

// Rollback applies compensating domain operations in one transaction. The
// immutable AuditEvent ledger is loaded before any inverse event is emitted and
// replayed newest-first. Revision/Snapshot and historical Claim rows are never
// deleted or rewritten.
func (s *RollbackService) Rollback(
	ctx context.Context,
	batchID, actorID uuid.UUID,
) (*RollbackResult, error) {
	actorType, status, err := s.repo.GetActor(ctx, nil, actorID)
	if err != nil || status != "active" {
		return nil, ErrInvalidActor
	}
	if actorType != "human" && actorType != "system" {
		return nil, ErrActorNotAllowed
	}
	result := newRollbackResult(batchID)

	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		batch, err := s.repo.GetChangeBatchForUpdate(ctx, tx, batchID)
		if err != nil {
			return err
		}
		if batch.Status == BatchRolledBack {
			result.Idempotent = true
			return nil
		}
		if batch.Status != BatchApplied {
			return fmt.Errorf(
				"%w: batch status=%s", ErrInvalidTransition, batch.Status,
			)
		}
		proposal, err := s.repo.GetProposalForUpdate(
			ctx, tx, batch.ProposalID,
		)
		if err != nil {
			return err
		}
		if proposal.Status != ProposalApplied {
			return ErrInvalidTransition
		}
		wikiID := wikiIDForProposal(ctx, tx, s.repo, proposal)
		if s.auth != nil {
			if err := s.auth.CheckTx(
				ctx, tx, actorID, wikiID,
				ActionBatchRollback, proposal.TargetID,
			); err != nil {
				return err
			}
		}

		events, err := s.repo.ListBatchAuditEvents(ctx, tx, batchID)
		if err != nil {
			return err
		}
		createdPages, err := createdPagesFromLedger(events)
		if err != nil {
			return err
		}
		revisions, err := s.repo.ListBatchRevisions(ctx, tx, batchID)
		if err != nil {
			return err
		}
		spans := buildRevisionSpans(revisions)
		if err := s.preflightRevisionSpans(
			ctx, tx, batchID, spans, createdPages,
		); err != nil {
			return err
		}
		batchClaimIDs, err := s.repo.ListBatchClaimIDs(
			ctx, tx, batchID,
		)
		if err != nil {
			return err
		}

		if err := s.repo.UpdateChangeBatchStatus(
			ctx, tx, batchID, BatchApplied, BatchRollbackPending,
		); err != nil {
			return err
		}

		for _, span := range spans {
			if createdPages[span.PageID] {
				continue
			}
			compensation, err := s.pages.RollbackInTx(
				ctx, tx, page.RollbackParams{
					PageID: span.PageID, TargetRevisionID: *span.FirstParentID,
					ActorID:       actorID,
					Summary:       "回滚 ChangeBatch " + batchID.String(),
					ChangeBatchID: &batchID,
				},
			)
			if err != nil {
				return err
			}
			result.RevisionIDs = append(
				result.RevisionIDs, compensation.ID,
			)
			result.CompensatedPageIDs = appendUniqueUUID(
				result.CompensatedPageIDs, span.PageID,
			)
		}

		handledClaims := make(map[uuid.UUID]struct{}, len(batchClaimIDs))
		for _, event := range events {
			if err := s.applyAuditInverse(
				ctx, tx, event, batchID, actorID, wikiID,
				handledClaims, result,
			); err != nil {
				return err
			}
		}
		for _, claimID := range batchClaimIDs {
			if _, handled := handledClaims[claimID]; !handled {
				return fmt.Errorf(
					"%w: batch claim=%s 缺少可重放审计事件",
					ErrUnsupportedOperation, claimID,
				)
			}
		}

		auditID, err := s.ids.New()
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"proposal_id": proposal.ID, "change_batch_id": batchID,
			"revision_count":             len(result.RevisionIDs),
			"claim_compensation_count":   len(result.CompensationClaimIDs),
			"deleted_page_count":         len(result.DeletedPageIDs),
			"deleted_entity_count":       len(result.DeletedEntityIDs),
			"entity_merge_count":         len(result.EntityMergeIDs),
			"collection_count":           len(result.CollectionIDs),
			"removed_claim_source_count": result.RemovedClaimSourceCount,
		})
		if err := s.repo.InsertAudit(
			ctx, tx, auditID, actorID, "proposal.rolled_back",
			"proposal", proposal.ID, batchID, payload,
		); err != nil {
			return err
		}
		if err := s.repo.UpdateChangeBatchStatus(
			ctx, tx, batchID, BatchRollbackPending, BatchRolledBack,
		); err != nil {
			return err
		}
		return s.repo.UpdateProposalStatus(
			ctx, tx, proposal.ID, ProposalApplied, ProposalRolledBack,
		)
	})
	if err != nil {
		return nil, normalizeRollbackError(err)
	}
	return result, nil
}

func newRollbackResult(batchID uuid.UUID) *RollbackResult {
	return &RollbackResult{
		ChangeBatchID: batchID,
		RevisionIDs:   []uuid.UUID{}, CompensationClaimIDs: []uuid.UUID{},
		CompensatedPageIDs: []uuid.UUID{}, DeletedPageIDs: []uuid.UUID{},
		DeletedEntityIDs: []uuid.UUID{}, RestoredEntityIDs: []uuid.UUID{},
		EntityMergeIDs: []uuid.UUID{}, CollectionIDs: []uuid.UUID{},
	}
}

func createdPagesFromLedger(
	events []BatchAuditEvent,
) (map[uuid.UUID]bool, error) {
	created := make(map[uuid.UUID]bool)
	for _, event := range events {
		if event.EventType != page.EventTypePageCreated {
			continue
		}
		if event.AggregateType != page.AggregateTypePage ||
			event.AggregateID == uuid.Nil {
			return nil, fmt.Errorf(
				"%w: invalid page.created ledger event=%s",
				ErrUnsupportedOperation, event.ID,
			)
		}
		created[event.AggregateID] = true
	}
	return created, nil
}

func buildRevisionSpans(revisions []BatchRevision) []rollbackRevisionSpan {
	index := make(map[uuid.UUID]int)
	spans := make([]rollbackRevisionSpan, 0)
	for _, revision := range revisions {
		i, exists := index[revision.PageID]
		if !exists {
			index[revision.PageID] = len(spans)
			spans = append(spans, rollbackRevisionSpan{
				PageID:        revision.PageID,
				FirstParentID: revision.ParentRevisionID,
				LastID:        revision.ID,
			})
			continue
		}
		spans[i].LastID = revision.ID
	}
	return spans
}

func (s *RollbackService) preflightRevisionSpans(
	ctx context.Context,
	tx pgx.Tx,
	batchID uuid.UUID,
	spans []rollbackRevisionSpan,
	createdPages map[uuid.UUID]bool,
) error {
	for _, span := range spans {
		current, err := s.repo.CurrentPageRevisionID(
			ctx, tx, span.PageID,
		)
		if err != nil {
			return err
		}
		later, err := s.repo.HasPageRevisionAfterBatch(
			ctx, tx, span.PageID, span.LastID, batchID,
		)
		if err != nil {
			return err
		}
		if current == nil || *current != span.LastID || later {
			return fmt.Errorf(
				"%w: page=%s batch_revision=%s current=%v later=%t",
				ErrRollbackStale, span.PageID, span.LastID, current, later,
			)
		}
		if createdPages[span.PageID] {
			if span.FirstParentID != nil {
				return fmt.Errorf(
					"%w: created page=%s has parent revision",
					ErrRollbackStale, span.PageID,
				)
			}
			continue
		}
		if span.FirstParentID == nil {
			return fmt.Errorf(
				"%w: page=%s batch has no pre-batch revision",
				ErrRollbackStale, span.PageID,
			)
		}
	}
	return nil
}

func (s *RollbackService) applyAuditInverse(
	ctx context.Context,
	tx pgx.Tx,
	event BatchAuditEvent,
	batchID, actorID, wikiID uuid.UUID,
	handledClaims map[uuid.UUID]struct{},
	result *RollbackResult,
) error {
	switch event.EventType {
	case "proposal.applied", page.EventTypeRevisionPublished:
		return nil
	case page.EventTypePageCreated:
		if event.AggregateType != page.AggregateTypePage {
			return invalidLedger(event)
		}
		if err := s.pages.RollbackCreatedPageInTx(
			ctx, tx, event.AggregateID, actorID, batchID,
		); err != nil {
			return err
		}
		result.DeletedPageIDs = appendUniqueUUID(
			result.DeletedPageIDs, event.AggregateID,
		)
		return nil
	case page.EventTypePageRenamed:
		return s.rollbackRenameEvent(
			ctx, tx, event, batchID, actorID, result,
		)
	case page.EventTypePageRedirected:
		return s.rollbackRedirectEvent(
			ctx, tx, event, batchID, actorID, result,
		)
	case "entity.created":
		return s.rollbackEntityCreatedEvent(
			ctx, tx, event, batchID, actorID, result,
		)
	case knowledge.OutboxEventEntityMerged:
		return s.rollbackEntityMergeEvent(
			ctx, tx, event, batchID, actorID, result,
		)
	case knowledge.OutboxEventClaimChanged:
		return s.rollbackClaimEvent(
			ctx, tx, event, batchID, actorID, handledClaims, result,
		)
	case collection.OutboxEventMembershipAdded,
		collection.OutboxEventMembershipRemoved:
		return s.rollbackCollectionEvent(
			ctx, tx, event, batchID, actorID, wikiID, result,
		)
	default:
		return fmt.Errorf(
			"%w: batch audit event_type=%q has no inverse",
			ErrUnsupportedOperation, event.EventType,
		)
	}
}

type pageRenameLedger struct {
	PageID             uuid.UUID `json:"page_id"`
	NormalizedTitle    string    `json:"normalized_title"`
	DisplayTitle       string    `json:"display_title"`
	OldNormalizedTitle string    `json:"old_normalized_title"`
	OldDisplayTitle    string    `json:"old_display_title"`
}

func (s *RollbackService) rollbackRenameEvent(
	ctx context.Context,
	tx pgx.Tx,
	event BatchAuditEvent,
	batchID, actorID uuid.UUID,
	result *RollbackResult,
) error {
	if event.AggregateType != page.AggregateTypePage {
		return invalidLedger(event)
	}
	var payload pageRenameLedger
	if err := decodeLedger(event, &payload); err != nil {
		return err
	}
	if payload.PageID == uuid.Nil {
		payload.PageID = event.AggregateID
	}
	if payload.PageID != event.AggregateID ||
		payload.NormalizedTitle == "" || payload.DisplayTitle == "" ||
		payload.OldNormalizedTitle == "" || payload.OldDisplayTitle == "" {
		return invalidLedger(event)
	}
	if _, err := s.pages.RollbackRenameInTx(
		ctx, tx, payload.PageID, payload.NormalizedTitle,
		payload.DisplayTitle, payload.OldDisplayTitle,
		actorID, batchID,
	); err != nil {
		return err
	}
	result.CompensatedPageIDs = appendUniqueUUID(
		result.CompensatedPageIDs, payload.PageID,
	)
	return nil
}

type pageRedirectLedger struct {
	SourcePageID   uuid.UUID            `json:"source_page_id"`
	Target         *page.RedirectTarget `json:"target"`
	PreviousTarget *page.RedirectTarget `json:"previous_target"`
}

func (s *RollbackService) rollbackRedirectEvent(
	ctx context.Context,
	tx pgx.Tx,
	event BatchAuditEvent,
	batchID, actorID uuid.UUID,
	result *RollbackResult,
) error {
	if event.AggregateType != page.AggregateTypePage {
		return invalidLedger(event)
	}
	var payload pageRedirectLedger
	if err := decodeLedger(event, &payload); err != nil {
		return err
	}
	if payload.SourcePageID == uuid.Nil {
		payload.SourcePageID = event.AggregateID
	}
	if payload.SourcePageID != event.AggregateID ||
		payload.Target == nil || payload.Target.Kind == "" {
		return invalidLedger(event)
	}
	if err := s.pages.RollbackRedirectInTx(
		ctx, tx, payload.SourcePageID, *payload.Target,
		payload.PreviousTarget, actorID, batchID,
	); err != nil {
		return err
	}
	result.CompensatedPageIDs = appendUniqueUUID(
		result.CompensatedPageIDs, payload.SourcePageID,
	)
	return nil
}

type entityCreatedLedger struct {
	EntityID          uuid.UUID `json:"entity_id"`
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
}

func (s *RollbackService) rollbackEntityCreatedEvent(
	ctx context.Context,
	tx pgx.Tx,
	event BatchAuditEvent,
	batchID, actorID uuid.UUID,
	result *RollbackResult,
) error {
	if event.AggregateType != "entity" {
		return invalidLedger(event)
	}
	var payload entityCreatedLedger
	if err := decodeLedger(event, &payload); err != nil {
		return err
	}
	if payload.EntityID == uuid.Nil {
		payload.EntityID = event.AggregateID
	}
	if payload.EntityID != event.AggregateID ||
		payload.ExpectedUpdatedAt.IsZero() || s.knowledge == nil {
		return invalidLedger(event)
	}
	if err := s.knowledge.RollbackCreatedEntityInTx(
		ctx, tx, payload.EntityID, actorID, batchID,
		payload.ExpectedUpdatedAt,
	); err != nil {
		return err
	}
	if err := s.insertCompensationAudit(
		ctx, tx, actorID, batchID, "entity.deleted", "entity",
		payload.EntityID, map[string]any{
			"entity_id":            payload.EntityID,
			"compensates_event_id": event.ID,
		},
	); err != nil {
		return err
	}
	result.DeletedEntityIDs = appendUniqueUUID(
		result.DeletedEntityIDs, payload.EntityID,
	)
	return nil
}

type entityMergeLedger struct {
	MergeID uuid.UUID `json:"merge_id"`
}

func (s *RollbackService) rollbackEntityMergeEvent(
	ctx context.Context,
	tx pgx.Tx,
	event BatchAuditEvent,
	batchID, actorID uuid.UUID,
	result *RollbackResult,
) error {
	if event.AggregateType != "entity_merge" || s.knowledge == nil {
		return invalidLedger(event)
	}
	var payload entityMergeLedger
	if err := decodeLedger(event, &payload); err != nil {
		return err
	}
	if payload.MergeID == uuid.Nil {
		payload.MergeID = event.AggregateID
	}
	if payload.MergeID != event.AggregateID {
		return invalidLedger(event)
	}
	compensation, err := s.knowledge.RollbackEntityMergeInTx(
		ctx, tx, payload.MergeID, actorID, batchID,
	)
	if err != nil {
		return err
	}
	result.EntityMergeIDs = appendUniqueUUID(
		result.EntityMergeIDs, payload.MergeID,
	)
	result.RestoredEntityIDs = appendUniqueUUID(
		result.RestoredEntityIDs, compensation.RestoredEntityID,
	)
	result.CompensationClaimIDs = append(
		result.CompensationClaimIDs,
		compensation.CompensatedClaimIDs...,
	)
	return nil
}

type claimLedger struct {
	ClaimID       uuid.UUID  `json:"claim_id"`
	CitationID    *uuid.UUID `json:"citation_id"`
	OperationType string     `json:"operation_type"`
}

func (s *RollbackService) rollbackClaimEvent(
	ctx context.Context,
	tx pgx.Tx,
	event BatchAuditEvent,
	batchID, actorID uuid.UUID,
	handledClaims map[uuid.UUID]struct{},
	result *RollbackResult,
) error {
	if event.AggregateType != "claim" || s.knowledge == nil {
		return invalidLedger(event)
	}
	var payload claimLedger
	if err := decodeLedger(event, &payload); err != nil {
		return err
	}
	if payload.ClaimID == uuid.Nil {
		payload.ClaimID = event.AggregateID
	}
	if payload.ClaimID != event.AggregateID {
		return invalidLedger(event)
	}
	switch payload.OperationType {
	case OpCreateClaim, OpSupersedeClaim:
		if _, handled := handledClaims[payload.ClaimID]; handled {
			return nil
		}
		compensation, err := s.knowledge.RollbackClaimBatchInTx(
			ctx, tx, payload.ClaimID, actorID, batchID,
		)
		if err != nil {
			return err
		}
		for _, claimID := range compensation.ConsumedClaimIDs {
			handledClaims[claimID] = struct{}{}
		}
		result.CompensationClaimIDs = appendUniqueUUID(
			result.CompensationClaimIDs, compensation.Current.ID,
		)
		return s.insertCompensationAudit(
			ctx, tx, actorID, batchID, "claim.rolled_back", "claim",
			payload.ClaimID, map[string]any{
				"claim_id":             payload.ClaimID,
				"current_claim_id":     compensation.Current.ID,
				"consumed_claim_ids":   compensation.ConsumedClaimIDs,
				"compensates_event_id": event.ID,
			},
		)
	case OpAddClaimSource:
		if payload.CitationID == nil || *payload.CitationID == uuid.Nil {
			return invalidLedger(event)
		}
		if err := s.knowledge.RemoveClaimSourceInTx(
			ctx, tx, payload.ClaimID, *payload.CitationID, actorID,
		); err != nil {
			return err
		}
		result.RemovedClaimSourceCount++
		return s.insertCompensationAudit(
			ctx, tx, actorID, batchID, "claim.source_removed", "claim",
			payload.ClaimID, map[string]any{
				"claim_id":             payload.ClaimID,
				"citation_id":          *payload.CitationID,
				"compensates_event_id": event.ID,
			},
		)
	default:
		return invalidLedger(event)
	}
}

type collectionMembershipLedger struct {
	CollectionID     uuid.UUID  `json:"collection_id"`
	MemberType       string     `json:"member_type"`
	PageID           *uuid.UUID `json:"page_id"`
	EntityID         *uuid.UUID `json:"entity_id"`
	SortKey          string     `json:"sort_key"`
	SourceType       string     `json:"source_type"`
	SourceRevisionID *uuid.UUID `json:"source_revision_id"`
}

func (s *RollbackService) rollbackCollectionEvent(
	ctx context.Context,
	tx pgx.Tx,
	event BatchAuditEvent,
	batchID, actorID, wikiID uuid.UUID,
	result *RollbackResult,
) error {
	if event.AggregateType != "collection" || s.collections == nil {
		return invalidLedger(event)
	}
	var payload collectionMembershipLedger
	if err := decodeLedger(event, &payload); err != nil {
		return err
	}
	if payload.CollectionID == uuid.Nil {
		payload.CollectionID = event.AggregateID
	}
	if payload.SourceType == "" {
		payload.SourceType = collection.TypeManual
	}
	if payload.CollectionID != event.AggregateID ||
		payload.SourceType != collection.TypeManual ||
		payload.SortKey == "" || payload.SourceRevisionID == nil ||
		*payload.SourceRevisionID == uuid.Nil {
		return invalidLedger(event)
	}
	input := collection.MemberInput{
		MemberType: payload.MemberType, PageID: payload.PageID,
		EntityID: payload.EntityID, SortKey: payload.SortKey,
		SourceRevisionID: *payload.SourceRevisionID,
	}
	add := event.EventType == collection.OutboxEventMembershipRemoved
	member, err := s.collections.ApplyManualMemberInTx(
		ctx, tx, wikiID, payload.CollectionID, actorID,
		input, add, &batchID,
	)
	if err != nil {
		return err
	}
	if !sameMembershipLedger(member, payload) {
		return fmt.Errorf(
			"%w: collection=%s member metadata changed",
			ErrRollbackStale, payload.CollectionID,
		)
	}
	result.CollectionIDs = appendUniqueUUID(
		result.CollectionIDs, payload.CollectionID,
	)
	return nil
}

func sameMembershipLedger(
	member collection.Membership,
	payload collectionMembershipLedger,
) bool {
	return member.MemberType == payload.MemberType &&
		member.SourceType == payload.SourceType &&
		member.SortKey == payload.SortKey &&
		equalUUIDPointers(member.PageID, payload.PageID) &&
		equalUUIDPointers(member.EntityID, payload.EntityID) &&
		equalUUIDPointers(
			member.SourceRevisionID, payload.SourceRevisionID,
		)
}

func equalUUIDPointers(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func decodeLedger(event BatchAuditEvent, target any) error {
	if err := json.Unmarshal(event.Payload, target); err != nil {
		return fmt.Errorf(
			"%w: decode audit event=%s type=%s: %v",
			ErrUnsupportedOperation, event.ID, event.EventType, err,
		)
	}
	return nil
}

func invalidLedger(event BatchAuditEvent) error {
	return fmt.Errorf(
		"%w: invalid audit event=%s type=%s aggregate=%s/%s",
		ErrUnsupportedOperation, event.ID, event.EventType,
		event.AggregateType, event.AggregateID,
	)
}

func (s *RollbackService) insertCompensationAudit(
	ctx context.Context,
	tx pgx.Tx,
	actorID, batchID uuid.UUID,
	eventType, aggregateType string,
	aggregateID uuid.UUID,
	payload any,
) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	return s.repo.InsertAudit(
		ctx, tx, auditID, actorID, eventType,
		aggregateType, aggregateID, batchID, data,
	)
}

func normalizeRollbackError(err error) error {
	if err == nil || errors.Is(err, ErrRollbackStale) {
		return err
	}
	switch {
	case errors.Is(err, page.ErrPageLifecycleStale),
		errors.Is(err, page.ErrStaleRevision),
		errors.Is(err, page.ErrPageNotFound),
		errors.Is(err, knowledge.ErrEntityLifecycleStale),
		errors.Is(err, knowledge.ErrEntityMergeStale),
		errors.Is(err, knowledge.ErrEntityNotFound),
		errors.Is(err, knowledge.ErrEntityMerged),
		errors.Is(err, knowledge.ErrInvalidClaimTransition),
		errors.Is(err, knowledge.ErrClaimNotFound),
		errors.Is(err, knowledge.ErrClaimSourceNotFound),
		errors.Is(err, collection.ErrInvalidMember),
		errors.Is(err, collection.ErrNotFound),
		errors.Is(err, collection.ErrInvalidDefinition):
		return fmt.Errorf("%w: %v", ErrRollbackStale, err)
	default:
		return err
	}
}
