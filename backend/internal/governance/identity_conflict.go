package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
)

const (
	identityConflictPage   = "page_identity"
	identityConflictEntity = "entity_identity"
)

type createEntityIdentityPayload struct {
	TypeKey      string `json:"type_key"`
	CanonicalKey string `json:"canonical_key"`
	Labels       []struct {
		Language    string `json:"language"`
		Label       string `json:"label"`
		Description string `json:"description"`
		IsPrimary   bool   `json:"is_primary"`
	} `json:"labels"`
}

type plannedIdentity struct {
	value json.RawMessage
}

// detectIdentityConflicts checks immutable create operations against the
// current authoritative identity indexes. Import Proposals can legitimately
// become stale while waiting for review when another ChangeBatch creates the
// same title or canonical key; that is a merge conflict, not an internal error.
func (s *ConflictService) detectIdentityConflicts(
	ctx context.Context,
	p *Proposal,
	records []OperationRecord,
) ([]MergeConflict, error) {
	if p == nil || s.pages == nil || s.knowledge == nil {
		return nil, ErrInvalidProposal
	}
	var wikiID uuid.UUID
	if p.TargetType == TargetWiki {
		if p.TargetID == nil || *p.TargetID == uuid.Nil {
			return nil, ErrInvalidProposal
		}
		wikiID = *p.TargetID
	}
	pagePlans := make(map[string]plannedIdentity)
	entityPlans := make(map[string]plannedIdentity)
	conflicts := make([]MergeConflict, 0)

	for i := range records {
		record := records[i]
		if record.OperationType != OpCreatePage && record.OperationType != OpCreateEntity {
			continue
		}
		op, err := OperationFromRecord(&record)
		if err != nil {
			return nil, err
		}
		if op.Target.WikiID == nil || *op.Target.WikiID != wikiID {
			if op.Target.WikiID == nil || *op.Target.WikiID == uuid.Nil || wikiID != uuid.Nil {
				return nil, ErrInvalidOperation
			}
			wikiID = *op.Target.WikiID
		}

		switch record.OperationType {
		case OpCreatePage:
			if op.Target.PageID == nil || op.Target.NamespaceID == nil ||
				*op.Target.PageID == uuid.Nil || *op.Target.NamespaceID == uuid.Nil {
				return nil, ErrInvalidOperation
			}
			var payload createPageOperationPayload
			if err := decodeOperationPayload(op.Payload, &payload); err != nil {
				return nil, err
			}
			normalized, err := page.NormalizeTitle(payload.Title)
			if err != nil {
				return nil, fmt.Errorf("%w: create_page title: %v", ErrInvalidOperation, err)
			}
			key := op.Target.NamespaceID.String() + "\x00" + normalized
			proposed := mustIdentityJSON(map[string]any{
				"kind": identityConflictPage, "source": "proposal",
				"operation_sequence": record.Sequence, "planned_page_id": *op.Target.PageID,
				"namespace_id": *op.Target.NamespaceID, "title": payload.Title,
				"normalized_title": normalized,
			})
			if previous, duplicate := pagePlans[key]; duplicate {
				conflicts = append(conflicts, MergeConflict{
					ProposalID: p.ID, ConflictType: ConflictSemantic,
					CurrentValue: previous.value, ProposedValue: proposed, Status: ConflictOpen,
				})
				continue
			}
			pagePlans[key] = plannedIdentity{value: proposed}
			resolved, err := s.pages.ResolveTitleByNamespaceID(
				ctx, wikiID, *op.Target.NamespaceID, payload.Title,
			)
			if errors.Is(err, page.ErrPageNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			current := mustIdentityJSON(map[string]any{
				"kind": identityConflictPage, "source": "authoritative",
				"existing_page_id":  resolved.Page.ID,
				"namespace_id":      resolved.Page.NamespaceID,
				"title":             resolved.Page.DisplayTitle,
				"normalized_title":  resolved.Page.NormalizedTitle,
				"matched_via_alias": resolved.ViaAlias,
			})
			conflicts = append(conflicts, MergeConflict{
				ProposalID: p.ID, PageID: &resolved.Page.ID,
				ConflictType: ConflictSemantic, CurrentValue: current,
				ProposedValue: proposed, Status: ConflictOpen,
			})

		case OpCreateEntity:
			if op.Target.EntityID == nil || *op.Target.EntityID == uuid.Nil {
				return nil, ErrInvalidOperation
			}
			var payload createEntityIdentityPayload
			if err := decodeOperationPayload(op.Payload, &payload); err != nil {
				return nil, err
			}
			rawKey := strings.TrimSpace(payload.CanonicalKey)
			if rawKey == "" {
				for _, label := range payload.Labels {
					if label.IsPrimary {
						rawKey = label.Label
						break
					}
				}
			}
			normalized, err := page.NormalizeTitle(rawKey)
			if err != nil {
				return nil, fmt.Errorf("%w: create_entity canonical_key: %v", ErrInvalidOperation, err)
			}
			proposed := mustIdentityJSON(map[string]any{
				"kind": identityConflictEntity, "source": "proposal",
				"operation_sequence": record.Sequence, "planned_entity_id": *op.Target.EntityID,
				"type_key": payload.TypeKey, "canonical_key": normalized,
			})
			if previous, duplicate := entityPlans[normalized]; duplicate {
				conflicts = append(conflicts, MergeConflict{
					ProposalID: p.ID, ConflictType: ConflictSemantic,
					CurrentValue: previous.value, ProposedValue: proposed, Status: ConflictOpen,
				})
				continue
			}
			entityPlans[normalized] = plannedIdentity{value: proposed}
			entity, err := s.knowledge.GetEntityByCanonicalKey(ctx, wikiID, rawKey)
			if errors.Is(err, knowledge.ErrEntityNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			current := mustIdentityJSON(map[string]any{
				"kind": identityConflictEntity, "source": "authoritative",
				"existing_entity_id": entity.ID, "canonical_key": entity.CanonicalKey,
				"entity_type_id": entity.EntityTypeID, "status": entity.Status,
			})
			conflicts = append(conflicts, MergeConflict{
				ProposalID: p.ID, ConflictType: ConflictSemantic,
				CurrentValue: current, ProposedValue: proposed, Status: ConflictOpen,
			})
		}
	}
	return conflicts, nil
}

func mustIdentityJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

func isIdentityConflict(conflict MergeConflict) bool {
	if conflict.ConflictType != ConflictSemantic {
		return false
	}
	var value struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(conflict.ProposedValue, &value); err != nil {
		return false
	}
	return value.Kind == identityConflictPage || value.Kind == identityConflictEntity
}

func identityConflictError(conflicts []MergeConflict) error {
	count := identityConflictCount(conflicts)
	return fmt.Errorf("%w（%d 项）；请重新运行对应导入生成基于 Current 的新提案", ErrIdentityConflict, count)
}

func identityConflictCount(conflicts []MergeConflict) int {
	count := 0
	for _, conflict := range conflicts {
		if isIdentityConflict(conflict) {
			count++
		}
	}
	return count
}

func isIdentityWriteConflict(err error) bool {
	return errors.Is(err, page.ErrTitleConflict) ||
		errors.Is(err, knowledge.ErrDuplicateEntityKey) ||
		errors.Is(err, knowledge.ErrBindingExists)
}
