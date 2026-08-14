package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/collection"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var ErrMergeConflict = errors.New("governance: Proposal 存在合并冲突")

type ApplyService struct {
	repo        *Repository
	pages       *page.Service
	pagePatch   *PagePatchEngine
	knowledge   *KnowledgePatchEngine
	collections *collection.Service
	conflicts   *ConflictService
	txm         *db.TxManager
	ids         *id.Generator
	auth        *AuthorizationService
}

func (s *ApplyService) WithAuthorization(auth *AuthorizationService) *ApplyService {
	s.auth = auth
	return s
}

func (s *ApplyService) WithCollections(service *collection.Service) *ApplyService {
	s.collections = service
	return s
}

func (s *ApplyService) Conflicts() *ConflictService {
	return s.conflicts
}

func NewApplyService(repo *Repository, pages *page.Service, pagePatch *PagePatchEngine,
	knowledge *KnowledgePatchEngine, conflicts *ConflictService, txm *db.TxManager, ids *id.Generator) *ApplyService {
	return &ApplyService{repo: repo, pages: pages, pagePatch: pagePatch,
		knowledge: knowledge, conflicts: conflicts, txm: txm, ids: ids}
}

type ApplyResult struct {
	ProposalID     uuid.UUID   `json:"proposal_id"`
	ChangeBatchID  uuid.UUID   `json:"change_batch_id"`
	RevisionID     *uuid.UUID  `json:"revision_id,omitempty"`
	RevisionIDs    []uuid.UUID `json:"revision_ids"`
	PageIDs        []uuid.UUID `json:"page_ids"`
	EntityIDs      []uuid.UUID `json:"entity_ids"`
	EntityMergeIDs []uuid.UUID `json:"entity_merge_ids"`
	CollectionIDs  []uuid.UUID `json:"collection_ids"`
	ClaimIDs       []uuid.UUID `json:"claim_ids"`
	Idempotent     bool        `json:"idempotent"`
}

type preparedPageContent struct {
	PageID            uuid.UUID
	AST               json.RawMessage
	ExpectedRevision  *uuid.UUID
	ContentOperations []OperationV1
}

// Apply 是 Proposal 生效的唯一治理入口。Conflict 检测通过后，ChangeBatch、
// Page/Knowledge 领域写入、Audit、Outbox 与 Proposal 状态在单事务提交。
func (s *ApplyService) Apply(ctx context.Context, proposalID, actorID uuid.UUID) (*ApplyResult, error) {
	actorType, status, err := s.repo.GetActor(ctx, nil, actorID)
	if err != nil || status != "active" {
		return nil, ErrInvalidActor
	}
	if actorType != "human" && actorType != "system" && actorType != "bot" {
		return nil, ErrActorNotAllowed
	}
	p, err := s.repo.GetProposal(ctx, nil, proposalID)
	if err != nil {
		return nil, err
	}
	if s.auth != nil {
		if err := s.auth.Check(ctx, actorID, wikiIDForProposal(ctx, nil, s.repo, p), ActionApply, p.TargetID); err != nil {
			return nil, err
		}
	}
	if existing, err := s.repo.GetChangeBatchByProposal(ctx, nil, proposalID); err != nil {
		return nil, err
	} else if existing != nil && (existing.Status == BatchApplied || existing.Status == BatchRolledBack) {
		return &ApplyResult{
			ProposalID: proposalID, ChangeBatchID: existing.ID,
			RevisionIDs: []uuid.UUID{}, PageIDs: []uuid.UUID{}, EntityIDs: []uuid.UUID{},
			EntityMergeIDs: []uuid.UUID{}, CollectionIDs: []uuid.UUID{},
			ClaimIDs: []uuid.UUID{}, Idempotent: true,
		}, nil
	}
	records, err := s.repo.ListOperations(ctx, nil, proposalID)
	if err != nil {
		return nil, err
	}
	ops := make([]OperationV1, len(records))
	for i := range records {
		op, err := OperationFromRecord(&records[i])
		if err != nil {
			return nil, err
		}
		ops[i] = *op
	}
	if err := s.validateSingleValueClaimOperations(ctx, nil, ops); err != nil {
		return nil, err
	}
	if err := s.validateWikiOperationTargets(ctx, nil, p, ops); err != nil {
		return nil, err
	}
	if s.conflicts != nil {
		conflicts, err := s.conflicts.DetectAndRecord(ctx, proposalID)
		if err != nil {
			return nil, err
		}
		if len(conflicts) > 0 {
			return nil, ErrMergeConflict
		}
	}

	contentOps := make([]OperationV1, 0, len(ops))
	for i := range ops {
		if isPageContentOperation(ops[i].OperationType) {
			contentOps = append(contentOps, ops[i])
		}
	}
	preparedPages, err := s.preparePageContent(ctx, p, contentOps)
	if err != nil {
		return nil, err
	}

	result := &ApplyResult{
		ProposalID: proposalID, RevisionIDs: []uuid.UUID{}, PageIDs: []uuid.UUID{},
		EntityIDs: []uuid.UUID{}, EntityMergeIDs: []uuid.UUID{},
		CollectionIDs: []uuid.UUID{}, ClaimIDs: []uuid.UUID{},
	}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		locked, err := s.repo.GetProposalForUpdate(ctx, tx, proposalID)
		if err != nil {
			return err
		}
		if batch, err := s.repo.GetChangeBatchByProposal(ctx, tx, proposalID); err != nil {
			return err
		} else if batch != nil {
			result.ChangeBatchID = batch.ID
			result.Idempotent = batch.Status == BatchApplied || batch.Status == BatchRolledBack
			if result.Idempotent {
				return nil
			}
			return ErrInvalidTransition
		}
		if locked.Status != ProposalApproved {
			return fmt.Errorf("%w: apply status=%s", ErrInvalidTransition, locked.Status)
		}
		approved, err := s.repo.HasApprovalEvidence(ctx, tx, proposalID)
		if err != nil {
			return err
		}
		if !approved {
			return ErrApprovalRequired
		}
		batchID, err := s.ids.New()
		if err != nil {
			return err
		}
		batch := &ChangeBatch{ID: batchID, ImportJobID: locked.ImportJobID,
			ProposalID: proposalID, ActorID: actorID, Status: BatchApplying}
		if err := s.repo.InsertChangeBatch(ctx, tx, batch); err != nil {
			return err
		}
		result.ChangeBatchID = batchID
		if err := s.repo.UpdateProposalStatus(ctx, tx, proposalID, ProposalApproved, ProposalApplying); err != nil {
			return err
		}

		if locked.TargetType == TargetPage {
			for i := range ops {
				switch {
				case isPageLifecycleOperation(ops[i].OperationType):
					applied, err := s.applyPageLifecycleInTx(
						ctx, tx, locked, ops[i], actorID, batchID,
					)
					if err != nil {
						return err
					}
					if applied.PageID != nil {
						result.PageIDs = appendUniqueUUID(result.PageIDs, *applied.PageID)
					}
					if applied.RevisionID != nil {
						result.RevisionID = applied.RevisionID
						result.RevisionIDs = appendUniqueUUID(result.RevisionIDs, *applied.RevisionID)
					}
				case isPageContentOperation(ops[i].OperationType):
					// Content operations are applied as one immutable Revision below.
				default:
					return fmt.Errorf("%w: page target operation=%s", ErrUnsupportedOperation, ops[i].OperationType)
				}
			}
			if len(contentOps) > 0 {
				prepared, ok := preparedPages[*locked.TargetID]
				if !ok {
					return ErrInvalidProposal
				}
				rev, err := s.pages.PublishInTx(ctx, tx, page.PublishParams{
					PageID: *locked.TargetID, ActorID: actorID, ExpectedRevisionID: prepared.ExpectedRevision,
					AST: prepared.AST, Summary: "Apply Proposal " + proposalID.String(), ChangeBatchID: &batchID,
				})
				if err != nil {
					return err
				}
				result.RevisionID = &rev.ID
				result.RevisionIDs = appendUniqueUUID(result.RevisionIDs, rev.ID)
				result.PageIDs = appendUniqueUUID(result.PageIDs, *locked.TargetID)
			}
		} else if locked.TargetType == TargetWiki {
			if err := s.applyWikiOperationsInTx(
				ctx, tx, locked, ops, preparedPages, actorID, batchID, result,
			); err != nil {
				return err
			}
		} else if locked.TargetType == TargetCollection {
			for i := range ops {
				if !isCollectionOperation(ops[i].OperationType) {
					return fmt.Errorf(
						"%w: collection target operation=%s",
						ErrUnsupportedOperation, ops[i].OperationType,
					)
				}
				collectionID, err := s.applyCollectionOperationInTx(
					ctx, tx, locked, ops[i], actorID, batchID,
				)
				if err != nil {
					return err
				}
				result.CollectionIDs = appendUniqueUUID(
					result.CollectionIDs, collectionID,
				)
			}
		} else {
			if s.knowledge == nil {
				return fmt.Errorf("%w: knowledge patch 未装配", ErrUnsupportedOperation)
			}
			for i := range ops {
				if ops[i].OperationType == OpSetPageEntityBinding {
					return fmt.Errorf(
						"%w: page binding requires a wiki-target Proposal",
						ErrUnsupportedOperation,
					)
				}
				if ops[i].OperationType == OpMergeEntity && s.auth != nil {
					if err := s.auth.CheckTx(
						ctx, tx, actorID,
						wikiIDForProposal(ctx, tx, s.repo, locked),
						ActionEntityMerge, nil,
					); err != nil {
						return err
					}
				}
				applied, err := s.knowledge.ApplyOneInTx(ctx, tx, ops[i], actorID, &batchID)
				if err != nil {
					return err
				}
				if applied.EntityID != nil {
					result.EntityIDs = appendUniqueUUID(result.EntityIDs, *applied.EntityID)
				}
				if applied.OperationType == OpCreateEntity {
					if applied.EntityID == nil || applied.EntityUpdatedAt == nil {
						return fmt.Errorf(
							"%w: create_entity 未返回生命周期凭据",
							ErrUnsupportedOperation,
						)
					}
					if err := s.emitEntityCreatedAudit(
						ctx, tx, actorID, batchID, proposalID,
						*applied.EntityID, *applied.EntityUpdatedAt,
					); err != nil {
						return err
					}
				}
				if applied.EntityMergeID != nil {
					result.EntityMergeIDs = appendUniqueUUID(
						result.EntityMergeIDs, *applied.EntityMergeID,
					)
				}
				if applied.ClaimID != nil {
					if applied.SubjectEntityID == nil {
						return fmt.Errorf(
							"%w: Claim Operation 未返回 subject_entity_id",
							ErrUnsupportedOperation,
						)
					}
					if ops[i].OperationType == OpCreateClaim || ops[i].OperationType == OpSupersedeClaim {
						if err := s.knowledge.PublishAppliedClaimInTx(ctx, tx, *applied.ClaimID); err != nil {
							return err
						}
					}
					result.ClaimIDs = append(result.ClaimIDs, *applied.ClaimID)
					if err := s.emitKnowledgeEvents(
						ctx, tx, actorID, batchID, proposalID,
						*applied.ClaimID, *applied.SubjectEntityID,
						applied.CitationID, ops[i].OperationType,
					); err != nil {
						return err
					}
				}
			}
		}
		if err := s.emitProposalAudit(ctx, tx, actorID, batchID, proposalID); err != nil {
			return err
		}
		if err := s.repo.UpdateChangeBatchStatus(ctx, tx, batchID, BatchApplying, BatchApplied); err != nil {
			return err
		}
		return s.repo.UpdateProposalStatus(ctx, tx, proposalID, ProposalApplying, ProposalApplied)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func appendUniqueUUID(values []uuid.UUID, value uuid.UUID) []uuid.UUID {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (s *ApplyService) preparePageContent(
	ctx context.Context,
	proposal *Proposal,
	contentOps []OperationV1,
) (map[uuid.UUID]preparedPageContent, error) {
	prepared := map[uuid.UUID]preparedPageContent{}
	if len(contentOps) == 0 {
		return prepared, nil
	}
	if proposal == nil || (proposal.TargetType != TargetPage && proposal.TargetType != TargetWiki) {
		return nil, ErrInvalidProposal
	}
	grouped := map[uuid.UUID][]OperationV1{}
	for _, op := range contentOps {
		if op.Target.PageID == nil || *op.Target.PageID == uuid.Nil {
			return nil, ErrInvalidOperation
		}
		if proposal.TargetType == TargetPage &&
			(proposal.TargetID == nil || *proposal.TargetID != *op.Target.PageID) {
			return nil, ErrInvalidOperation
		}
		grouped[*op.Target.PageID] = append(grouped[*op.Target.PageID], op)
	}
	pageIDs := make([]uuid.UUID, 0, len(grouped))
	for pageID := range grouped {
		pageIDs = append(pageIDs, pageID)
	}
	sort.Slice(pageIDs, func(i, j int) bool {
		return pageIDs[i].String() < pageIDs[j].String()
	})
	resolvedConflicts, err := s.repo.ListMergeConflicts(ctx, proposal.ID)
	if err != nil {
		return nil, err
	}
	for _, pageID := range pageIDs {
		if proposal.TargetType == TargetWiki {
			if proposal.TargetID == nil || *proposal.TargetID == uuid.Nil {
				return nil, ErrInvalidProposal
			}
			targetPage, err := s.pages.GetPage(ctx, pageID)
			if err != nil {
				return nil, err
			}
			if targetPage.WikiID != *proposal.TargetID || targetPage.DeletedAt != nil {
				return nil, fmt.Errorf(
					"%w: page=%s 不属于 Proposal wiki=%s",
					ErrInvalidOperation, pageID, *proposal.TargetID,
				)
			}
		}
		currentRev, currentSnap, err := s.pages.CurrentContent(ctx, pageID)
		if err != nil {
			return nil, fmt.Errorf("governance: 读取 Apply Current 失败: %w", err)
		}
		if currentSnap == nil {
			currentSnap = &page.ContentSnapshot{AST: mustEmptyDocumentJSON()}
		}
		currentDoc, err := ast.Parse(currentSnap.AST)
		if err != nil {
			return nil, err
		}
		effectiveOps, err := applyConflictResolutions(
			currentDoc, grouped[pageID], resolvedConflicts,
		)
		if err != nil {
			return nil, err
		}
		proposed, err := s.pagePatch.Apply(currentDoc, pageID, effectiveOps)
		if err != nil {
			return nil, err
		}
		pageAST, err := ast.CanonicalJSON(proposed)
		if err != nil {
			return nil, err
		}
		var expected *uuid.UUID
		if currentRev != nil {
			value := currentRev.ID
			expected = &value
		}
		prepared[pageID] = preparedPageContent{
			PageID: pageID, AST: pageAST, ExpectedRevision: expected,
			ContentOperations: effectiveOps,
		}
	}
	return prepared, nil
}

func mustEmptyDocumentJSON() json.RawMessage {
	raw, _ := ast.CanonicalJSON(ast.NewDocument())
	return raw
}

func (s *ApplyService) applyWikiOperationsInTx(
	ctx context.Context,
	tx pgx.Tx,
	proposal *Proposal,
	ops []OperationV1,
	preparedPages map[uuid.UUID]preparedPageContent,
	actorID, batchID uuid.UUID,
	result *ApplyResult,
) error {
	if proposal == nil || proposal.TargetID == nil || *proposal.TargetID == uuid.Nil {
		return ErrInvalidProposal
	}
	if s.knowledge == nil {
		return fmt.Errorf("%w: knowledge patch 未装配", ErrUnsupportedOperation)
	}
	if err := s.validateWikiOperationTargets(ctx, tx, proposal, ops); err != nil {
		return err
	}
	createdPageIDs := map[uuid.UUID]bool{}
	for i := range ops {
		switch {
		case isPageLifecycleOperation(ops[i].OperationType):
			applied, err := s.applyPageLifecycleInTx(
				ctx, tx, proposal, ops[i], actorID, batchID,
			)
			if err != nil {
				return err
			}
			if applied.PageID != nil {
				result.PageIDs = appendUniqueUUID(result.PageIDs, *applied.PageID)
			}
			if applied.RevisionID != nil {
				result.RevisionID = applied.RevisionID
				result.RevisionIDs = appendUniqueUUID(result.RevisionIDs, *applied.RevisionID)
			}
			if ops[i].OperationType == OpCreatePage && applied.PageID != nil {
				createdPageIDs[*applied.PageID] = true
			}
		case isPageContentOperation(ops[i].OperationType):
			// One immutable Revision per target page is published after all
			// lifecycle and knowledge dependencies have been applied.
		case isKnowledgeOperation(ops[i].OperationType):
			if ops[i].OperationType == OpMergeEntity && s.auth != nil {
				if err := s.auth.CheckTx(
					ctx, tx, actorID, *proposal.TargetID,
					ActionEntityMerge, nil,
				); err != nil {
					return err
				}
			}
			if err := s.applyKnowledgeOperationInTx(
				ctx, tx, proposal.ID, ops[i], actorID, batchID, result,
			); err != nil {
				return err
			}
		default:
			return fmt.Errorf(
				"%w: wiki target operation=%s",
				ErrUnsupportedOperation, ops[i].OperationType,
			)
		}
	}

	pageIDs := make([]uuid.UUID, 0, len(preparedPages))
	for pageID := range preparedPages {
		pageIDs = append(pageIDs, pageID)
	}
	sort.Slice(pageIDs, func(i, j int) bool {
		return pageIDs[i].String() < pageIDs[j].String()
	})
	for _, pageID := range pageIDs {
		if createdPageIDs[pageID] {
			return fmt.Errorf(
				"%w: create_page initial_ast and page content operations target the same page=%s",
				ErrInvalidOperation, pageID,
			)
		}
		prepared := preparedPages[pageID]
		if s.auth != nil {
			if err := s.auth.CheckTx(
				ctx, tx, actorID, *proposal.TargetID, ActionEdit, &pageID,
			); err != nil {
				return err
			}
		}
		revision, err := s.pages.PublishInTx(ctx, tx, page.PublishParams{
			PageID: pageID, ActorID: actorID,
			ExpectedRevisionID: prepared.ExpectedRevision,
			AST:                prepared.AST,
			Summary:            "Apply Proposal " + proposal.ID.String(),
			ChangeBatchID:      &batchID,
		})
		if err != nil {
			return err
		}
		result.RevisionID = &revision.ID
		result.RevisionIDs = appendUniqueUUID(result.RevisionIDs, revision.ID)
		result.PageIDs = appendUniqueUUID(result.PageIDs, pageID)
	}
	return nil
}

func (s *ApplyService) validateWikiOperationTargets(
	ctx context.Context,
	tx pgx.Tx,
	proposal *Proposal,
	ops []OperationV1,
) error {
	if proposal == nil || proposal.TargetType != TargetWiki {
		return nil
	}
	if proposal.TargetID == nil || *proposal.TargetID == uuid.Nil || s.pages == nil || s.knowledge == nil {
		return ErrInvalidProposal
	}
	wikiID := *proposal.TargetID
	plannedEntityIDs := make(map[uuid.UUID]struct{})
	plannedPageIDs := make(map[uuid.UUID]struct{})
	for i := range ops {
		switch ops[i].OperationType {
		case OpCreateEntity:
			if ops[i].Target.EntityID == nil || *ops[i].Target.EntityID == uuid.Nil ||
				ops[i].Target.WikiID == nil || *ops[i].Target.WikiID != wikiID {
				return ErrInvalidOperation
			}
			if _, duplicate := plannedEntityIDs[*ops[i].Target.EntityID]; duplicate {
				return fmt.Errorf(
					"%w: duplicate create_entity=%s",
					ErrInvalidOperation, *ops[i].Target.EntityID,
				)
			}
			plannedEntityIDs[*ops[i].Target.EntityID] = struct{}{}
		case OpCreatePage:
			if ops[i].Target.PageID == nil || *ops[i].Target.PageID == uuid.Nil {
				return ErrInvalidOperation
			}
			if _, duplicate := plannedPageIDs[*ops[i].Target.PageID]; duplicate {
				return fmt.Errorf(
					"%w: duplicate create_page=%s",
					ErrInvalidOperation, *ops[i].Target.PageID,
				)
			}
			plannedPageIDs[*ops[i].Target.PageID] = struct{}{}
		}
	}
	for i := range ops {
		op := ops[i]
		switch {
		case op.OperationType == OpCreatePage:
			if op.Target.WikiID == nil || *op.Target.WikiID != wikiID ||
				op.Target.PageID == nil || *op.Target.PageID == uuid.Nil {
				return ErrInvalidOperation
			}
		case isPageLifecycleOperation(op.OperationType), isPageContentOperation(op.OperationType):
			if op.Target.PageID == nil || *op.Target.PageID == uuid.Nil {
				return ErrInvalidOperation
			}
			targetPage, err := s.pages.GetPageInTx(ctx, tx, *op.Target.PageID)
			if err != nil {
				return err
			}
			if targetPage.WikiID != wikiID || targetPage.DeletedAt != nil {
				return fmt.Errorf(
					"%w: page=%s 不属于 Proposal wiki=%s",
					ErrInvalidOperation, *op.Target.PageID, wikiID,
				)
			}
		case isKnowledgeOperation(op.OperationType):
			if op.OperationType == OpSetPageEntityBinding {
				if op.Target.WikiID == nil || *op.Target.WikiID != wikiID ||
					op.Target.PageID == nil || op.Target.EntityID == nil {
					return ErrInvalidOperation
				}
				if _, planned := plannedPageIDs[*op.Target.PageID]; !planned {
					targetPage, err := s.pages.GetPageInTx(ctx, tx, *op.Target.PageID)
					if err != nil {
						return err
					}
					if targetPage.WikiID != wikiID || targetPage.DeletedAt != nil {
						return ErrInvalidOperation
					}
				}
			}
			if err := s.knowledge.ValidateWikiOperationInTx(
				ctx, tx, wikiID, op, plannedEntityIDs,
			); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: wiki target operation=%s", ErrUnsupportedOperation, op.OperationType)
		}
	}
	return nil
}

// validateSingleValueClaimOperations rejects an internally impossible
// Proposal before opening its atomic write transaction. Knowledge still owns
// and enforces cardinality at write time; this preflight prevents a reviewed
// batch from failing halfway through because it contains multiple new values
// for the same single-valued property.
func (s *ApplyService) validateSingleValueClaimOperations(
	ctx context.Context,
	tx pgx.Tx,
	ops []OperationV1,
) error {
	if s.knowledge == nil || s.knowledge.knowledge == nil {
		return nil
	}
	seen := make(map[string]int)
	for index := range ops {
		op := ops[index]
		if op.OperationType != OpCreateClaim && op.OperationType != OpSupersedeClaim {
			continue
		}
		payload, err := decodeClaimPayload(op.Payload)
		if err != nil {
			return fmt.Errorf("%w: claim operation %d payload", ErrInvalidOperation, index+1)
		}
		var subjectID uuid.UUID
		if op.OperationType == OpCreateClaim {
			if op.Target.EntityID == nil {
				return fmt.Errorf("%w: create_claim operation %d 缺少 subject", ErrInvalidOperation, index+1)
			}
			subjectID = *op.Target.EntityID
		} else {
			var envelope struct {
				SubjectEntityID uuid.UUID `json:"subject_entity_id"`
			}
			if err := json.Unmarshal(op.Payload, &envelope); err != nil {
				return fmt.Errorf("%w: supersede_claim operation %d payload", ErrInvalidOperation, index+1)
			}
			subjectID = envelope.SubjectEntityID
		}
		if subjectID == uuid.Nil {
			return fmt.Errorf("%w: claim operation %d subject 为空", ErrInvalidOperation, index+1)
		}
		property, err := s.knowledge.knowledge.GetPropertyByKeyInTx(ctx, tx, payload.PropertyKey)
		if err != nil {
			return err
		}
		if property.IsMultivalued {
			continue
		}
		key := subjectID.String() + "\x00" + property.PropertyKey
		if first, duplicate := seen[key]; duplicate {
			return fmt.Errorf(
				"%w: operations %d and %d both set single-valued property %q for subject=%s",
				ErrInvalidOperation, first+1, index+1, property.PropertyKey, subjectID,
			)
		}
		seen[key] = index
	}
	return nil
}

func isKnowledgeOperation(operationType string) bool {
	switch operationType {
	case OpCreateEntity, OpMergeEntity, OpCreateClaim,
		OpSupersedeClaim, OpAddClaimSource, OpSetPageEntityBinding:
		return true
	default:
		return false
	}
}

func (s *ApplyService) applyKnowledgeOperationInTx(
	ctx context.Context,
	tx pgx.Tx,
	proposalID uuid.UUID,
	op OperationV1,
	actorID, batchID uuid.UUID,
	result *ApplyResult,
) error {
	applied, err := s.knowledge.ApplyOneInTx(ctx, tx, op, actorID, &batchID)
	if err != nil {
		return err
	}
	if applied.EntityID != nil {
		result.EntityIDs = appendUniqueUUID(result.EntityIDs, *applied.EntityID)
	}
	if applied.PageID != nil {
		result.PageIDs = appendUniqueUUID(result.PageIDs, *applied.PageID)
	}
	if applied.OperationType == OpCreateEntity {
		if applied.EntityID == nil || applied.EntityUpdatedAt == nil {
			return fmt.Errorf(
				"%w: create_entity 未返回生命周期凭据",
				ErrUnsupportedOperation,
			)
		}
		if err := s.emitEntityCreatedAudit(
			ctx, tx, actorID, batchID, proposalID,
			*applied.EntityID, *applied.EntityUpdatedAt,
		); err != nil {
			return err
		}
	}
	if applied.EntityMergeID != nil {
		result.EntityMergeIDs = appendUniqueUUID(
			result.EntityMergeIDs, *applied.EntityMergeID,
		)
	}
	if applied.ClaimID != nil {
		if applied.SubjectEntityID == nil {
			return fmt.Errorf(
				"%w: Claim Operation 未返回 subject_entity_id",
				ErrUnsupportedOperation,
			)
		}
		if op.OperationType == OpCreateClaim || op.OperationType == OpSupersedeClaim {
			if err := s.knowledge.PublishAppliedClaimInTx(ctx, tx, *applied.ClaimID); err != nil {
				return err
			}
		}
		result.ClaimIDs = append(result.ClaimIDs, *applied.ClaimID)
		if err := s.emitKnowledgeEvents(
			ctx, tx, actorID, batchID, proposalID,
			*applied.ClaimID, *applied.SubjectEntityID,
			applied.CitationID, op.OperationType,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *ApplyService) emitProposalAudit(ctx context.Context, tx pgx.Tx, actorID, batchID, proposalID uuid.UUID) error {
	id, err := s.ids.New()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"proposal_id": proposalID, "change_batch_id": batchID})
	return s.repo.InsertAudit(ctx, tx, id, actorID, "proposal.applied", "proposal", proposalID, batchID, payload)
}

func (s *ApplyService) emitEntityCreatedAudit(
	ctx context.Context,
	tx pgx.Tx,
	actorID, batchID, proposalID, entityID uuid.UUID,
	expectedUpdatedAt time.Time,
) error {
	payload, _ := json.Marshal(map[string]any{
		"proposal_id": proposalID, "change_batch_id": batchID,
		"entity_id": entityID, "operation_type": OpCreateEntity,
		"expected_updated_at": expectedUpdatedAt,
	})
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	return s.repo.InsertAudit(
		ctx, tx, auditID, actorID, "entity.created", "entity",
		entityID, batchID, payload,
	)
}

func (s *ApplyService) emitKnowledgeEvents(
	ctx context.Context,
	tx pgx.Tx,
	actorID, batchID, proposalID, claimID, subjectEntityID uuid.UUID,
	citationID *uuid.UUID,
	operationType string,
) error {
	payload, _ := json.Marshal(map[string]any{
		"proposal_id": proposalID, "change_batch_id": batchID,
		"claim_id": claimID, "subject_entity_id": subjectEntityID,
		"citation_id":    citationID,
		"operation_type": operationType,
	})
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	if err := s.repo.InsertAudit(ctx, tx, auditID, actorID, "claim.changed", "claim", claimID, batchID, payload); err != nil {
		return err
	}
	outboxID, err := s.ids.New()
	if err != nil {
		return err
	}
	return s.repo.InsertOutbox(ctx, tx, outboxID, "claim", claimID, "claim.changed", payload)
}
