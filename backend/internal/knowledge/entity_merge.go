package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	EntityMergeApplied    = "applied"
	EntityMergeRolledBack = "rolled_back"

	OutboxEventEntityMerged = "entity.merged"

	MergeLabelCopied         = "copied"
	MergeLabelDemotedPrimary = "demoted_primary"
	MergeLabelExisting       = "existing"
)

type EntityMerge struct {
	ID             uuid.UUID
	SourceEntityID uuid.UUID
	TargetEntityID uuid.UUID
	ActorID        uuid.UUID
	Status         string
	Reason         string
	CreatedAt      time.Time
	RolledBackAt   *time.Time
	RolledBackBy   *uuid.UUID
}

type EntityMergeLabelMap struct {
	MergeID         uuid.UUID
	Language        string
	SourceLabel     string
	TargetLabel     string
	Action          string
	TargetIsPrimary bool
}

type EntityMergeClaimMap struct {
	MergeID    uuid.UUID
	OldClaimID uuid.UUID
	NewClaimID uuid.UUID
	OldStatus  string
	NewStatus  string
}

type MergeEntityParams struct {
	SourceEntityID uuid.UUID
	TargetEntityID uuid.UUID
	ActorID        uuid.UUID
	Reason         string
}

type MergeEntityResult struct {
	Merge         *EntityMerge
	LabelMappings []EntityMergeLabelMap
	ClaimMappings []EntityMergeClaimMap
	Idempotent    bool
}

type RollbackEntityMergeResult struct {
	MergeID             uuid.UUID
	RestoredEntityID    uuid.UUID
	CompensatedClaimIDs []uuid.UUID
	RemovedTargetLabels int
	Idempotent          bool
}

// ResolveEntity follows merged_into mappings and returns the active terminal Entity.
func (s *Service) ResolveEntity(ctx context.Context, entityID uuid.UUID) (*Entity, error) {
	seen := make(map[uuid.UUID]struct{})
	currentID := entityID
	for depth := 0; depth < 64; depth++ {
		if _, ok := seen[currentID]; ok {
			return nil, fmt.Errorf("%w: entity=%s", ErrEntityMergeCycle, currentID)
		}
		seen[currentID] = struct{}{}
		entity, err := s.repo.GetEntityByID(ctx, nil, currentID)
		if err != nil {
			return nil, err
		}
		switch entity.Status {
		case StatusActive:
			return entity, nil
		case StatusMerged:
			if entity.MergedIntoEntityID == nil {
				return nil, fmt.Errorf("%w: entity=%s has no target", ErrEntityMergeCycle, entity.ID)
			}
			currentID = *entity.MergedIntoEntityID
		default:
			return nil, fmt.Errorf("%w: entity=%s status=%s", ErrInvalidEntityMerge, entity.ID, entity.Status)
		}
	}
	return nil, fmt.Errorf("%w: chain exceeds 64 hops", ErrEntityMergeCycle)
}

// ResolveMergedClaim follows applied Entity merge mappings to the current Claim.
// A rolled-back merge is ignored, so the original Claim becomes current again.
func (s *Service) ResolveMergedClaim(ctx context.Context, claimID uuid.UUID) (*Claim, error) {
	seen := make(map[uuid.UUID]struct{})
	currentID := claimID
	for depth := 0; depth < 64; depth++ {
		if _, ok := seen[currentID]; ok {
			return nil, fmt.Errorf("%w: claim=%s", ErrEntityMergeCycle, currentID)
		}
		seen[currentID] = struct{}{}
		nextID, err := s.repo.getAppliedClaimMapping(ctx, nil, currentID)
		if err != nil {
			return nil, err
		}
		if nextID == nil {
			return s.repo.GetClaimByID(ctx, nil, currentID)
		}
		currentID = *nextID
	}
	return nil, fmt.Errorf("%w: Claim chain exceeds 64 hops", ErrEntityMergeCycle)
}

// MergeEntity atomically records an auditable merge, copies labels, clones current
// Claims to the target identity, and marks the source as merged.
func (s *Service) MergeEntity(ctx context.Context, params MergeEntityParams) (*MergeEntityResult, error) {
	if params.SourceEntityID == uuid.Nil || params.TargetEntityID == uuid.Nil ||
		params.SourceEntityID == params.TargetEntityID || params.ActorID == uuid.Nil ||
		len(strings.TrimSpace(params.Reason)) > 1000 {
		return nil, ErrInvalidEntityMerge
	}
	result := &MergeEntityResult{}
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.checkMergeActor(ctx, tx, params.ActorID); err != nil {
			return err
		}
		source, target, err := s.lockMergeEntities(ctx, tx, params.SourceEntityID, params.TargetEntityID)
		if err != nil {
			return err
		}
		if source.Status == StatusMerged {
			existing, err := s.repo.getAppliedMergeBySource(ctx, tx, source.ID)
			if err != nil {
				return err
			}
			if existing != nil && existing.TargetEntityID == target.ID {
				result.Merge = existing
				result.LabelMappings, err = s.repo.listMergeLabels(ctx, tx, existing.ID)
				if err != nil {
					return err
				}
				result.ClaimMappings, err = s.listMergeClaimMaps(ctx, tx, existing.ID)
				result.Idempotent = err == nil
				return err
			}
			return ErrInvalidEntityMerge
		}
		if source.Status != StatusActive || target.Status != StatusActive ||
			source.WikiID != target.WikiID || source.EntityTypeID != target.EntityTypeID {
			return ErrInvalidEntityMerge
		}

		mergeID, err := s.ids.New()
		if err != nil {
			return err
		}
		merge := &EntityMerge{
			ID: mergeID, SourceEntityID: source.ID, TargetEntityID: target.ID,
			ActorID: params.ActorID, Status: EntityMergeApplied,
			Reason: strings.TrimSpace(params.Reason),
		}
		if err := s.repo.insertEntityMerge(ctx, tx, merge); err != nil {
			return err
		}
		result.Merge = merge
		if result.LabelMappings, err = s.migrateMergeLabels(ctx, tx, merge); err != nil {
			return err
		}
		if result.ClaimMappings, err = s.migrateMergeClaims(ctx, tx, merge); err != nil {
			return err
		}
		if err := s.repo.markEntityMerged(ctx, tx, source.ID, target.ID); err != nil {
			return err
		}
		auditID, err := s.ids.New()
		if err != nil {
			return err
		}
		outboxID, err := s.ids.New()
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"merge_id": merge.ID, "actor_id": params.ActorID,
			"source_entity_id": source.ID, "target_entity_id": target.ID,
			"label_mapping_count": len(result.LabelMappings),
			"claim_mapping_count": len(result.ClaimMappings),
		})
		return s.repo.insertMergeEvent(ctx, tx, auditID, outboxID, params.ActorID,
			OutboxEventEntityMerged, merge.ID, payload)
	})
	return result, err
}

func (s *Service) checkMergeActor(ctx context.Context, tx pgx.Tx, actorID uuid.UUID) error {
	actorType, err := s.repo.getActorType(ctx, tx, actorID)
	if err != nil {
		return err
	}
	if actorType != "human" && actorType != "system" {
		return ErrEntityMergeActorOnly
	}
	return nil
}

func (s *Service) lockMergeEntities(
	ctx context.Context,
	tx pgx.Tx,
	sourceID, targetID uuid.UUID,
) (*Entity, *Entity, error) {
	firstID, secondID := sourceID, targetID
	if strings.Compare(firstID.String(), secondID.String()) > 0 {
		firstID, secondID = secondID, firstID
	}
	first, err := s.repo.GetEntityByIDForUpdate(ctx, tx, firstID)
	if err != nil {
		return nil, nil, err
	}
	second, err := s.repo.GetEntityByIDForUpdate(ctx, tx, secondID)
	if err != nil {
		return nil, nil, err
	}
	if first.ID == sourceID {
		return first, second, nil
	}
	return second, first, nil
}

func (s *Service) migrateMergeLabels(
	ctx context.Context,
	tx pgx.Tx,
	merge *EntityMerge,
) ([]EntityMergeLabelMap, error) {
	labels, err := s.repo.ListLabels(ctx, tx, merge.SourceEntityID)
	if err != nil {
		return nil, err
	}
	result := make([]EntityMergeLabelMap, 0, len(labels))
	for _, label := range labels {
		mapping := EntityMergeLabelMap{
			MergeID: merge.ID, Language: label.Language, SourceLabel: label.Label,
			TargetLabel: label.Label, TargetIsPrimary: label.IsPrimary,
		}
		if _, err := s.repo.GetLabel(ctx, tx, merge.TargetEntityID, label.Language, label.Label); err == nil {
			mapping.Action = MergeLabelExisting
			existing, _ := s.repo.GetLabel(ctx, tx, merge.TargetEntityID, label.Language, label.Label)
			mapping.TargetIsPrimary = existing.IsPrimary
		} else if !errors.Is(err, ErrLabelNotFound) {
			return nil, err
		} else {
			if label.IsPrimary {
				if _, err := s.repo.GetPrimaryLabel(ctx, tx, merge.TargetEntityID, label.Language); err == nil {
					label.IsPrimary = false
					mapping.TargetIsPrimary = false
					mapping.Action = MergeLabelDemotedPrimary
				} else if !errors.Is(err, ErrLabelNotFound) {
					return nil, err
				}
			}
			if mapping.Action == "" {
				mapping.Action = MergeLabelCopied
			}
			label.EntityID = merge.TargetEntityID
			if err := s.repo.InsertLabel(ctx, tx, &label); err != nil {
				return nil, err
			}
		}
		if err := s.repo.insertMergeLabelMap(ctx, tx, mapping); err != nil {
			return nil, err
		}
		result = append(result, mapping)
	}
	return result, nil
}

func (s *Service) migrateMergeClaims(
	ctx context.Context,
	tx pgx.Tx,
	merge *EntityMerge,
) ([]EntityMergeClaimMap, error) {
	claims, err := s.repo.listCurrentClaimsForMerge(ctx, tx, merge.SourceEntityID)
	if err != nil {
		return nil, err
	}
	result := make([]EntityMergeClaimMap, 0, len(claims))
	for i := range claims {
		old := &claims[i]
		newClaim := *old
		newClaim.ID, err = s.ids.New()
		if err != nil {
			return nil, err
		}
		newClaim.CreatedBy = merge.ActorID
		newClaim.CreatedAt = time.Time{}
		newClaim.ChangeBatchID = nil
		newClaim.SupersededBy = nil
		if newClaim.SubjectEntityID == merge.SourceEntityID {
			prop, err := s.repo.GetPropertyByID(ctx, tx, newClaim.PropertyID)
			if err != nil {
				return nil, err
			}
			if !prop.IsMultivalued && old.Status == ClaimStatusPublished {
				n, err := s.repo.CountPublishedClaims(ctx, tx, merge.TargetEntityID, prop.ID)
				if err != nil {
					return nil, err
				}
				if n > 0 {
					return nil, fmt.Errorf("%w: target has published single-value property %s",
						ErrInvalidEntityMerge, prop.PropertyKey)
				}
			}
			newClaim.SubjectEntityID = merge.TargetEntityID
		}
		if newClaim.TargetEntityID != nil && *newClaim.TargetEntityID == merge.SourceEntityID {
			target := merge.TargetEntityID
			newClaim.TargetEntityID = &target
			newClaim.ValueJSON, _ = json.Marshal(map[string]uuid.UUID{"entity_id": target})
		}
		compensated := mergeOldClaimStatus(old.Status)
		if err := s.repo.updateClaimMergeStatus(ctx, tx, old.ID, old.Status, compensated); err != nil {
			return nil, err
		}
		if err := s.repo.InsertClaim(ctx, tx, &newClaim); err != nil {
			return nil, err
		}
		sources, err := s.repo.listClaimSources(ctx, tx, old.ID)
		if err != nil {
			return nil, err
		}
		for _, source := range sources {
			if err := s.repo.copyClaimSource(ctx, tx, newClaim.ID, source); err != nil {
				return nil, err
			}
		}
		mapping := EntityMergeClaimMap{
			MergeID: merge.ID, OldClaimID: old.ID, NewClaimID: newClaim.ID,
			OldStatus: old.Status, NewStatus: newClaim.Status,
		}
		if err := s.repo.insertMergeClaimMap(ctx, tx, mapping); err != nil {
			return nil, err
		}
		if err := s.emitClaimChanged(ctx, tx, old.ID, old.SubjectEntityID, &newClaim.ID); err != nil {
			return nil, err
		}
		if err := s.emitClaimChanged(ctx, tx, newClaim.ID, newClaim.SubjectEntityID, nil); err != nil {
			return nil, err
		}
		result = append(result, mapping)
	}
	return result, nil
}

func mergeOldClaimStatus(status string) string {
	if status == ClaimStatusPublished {
		return ClaimStatusDeprecated
	}
	return ClaimStatusRejected
}

func (s *Service) listMergeClaimMaps(
	ctx context.Context,
	tx pgx.Tx,
	mergeID uuid.UUID,
) ([]EntityMergeClaimMap, error) {
	rows, err := s.repo.q(tx).Query(ctx, `SELECT merge_id,old_claim_id,new_claim_id,
		old_status,new_status FROM entity_merge_claim_map
		WHERE merge_id=$1 ORDER BY old_claim_id`, mergeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []EntityMergeClaimMap
	for rows.Next() {
		var item EntityMergeClaimMap
		if err := rows.Scan(&item.MergeID, &item.OldClaimID, &item.NewClaimID,
			&item.OldStatus, &item.NewStatus); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// RollbackEntityMerge compensates an applied merge without deleting audit history.
func (s *Service) RollbackEntityMerge(
	ctx context.Context,
	mergeID, actorID uuid.UUID,
) (*RollbackEntityMergeResult, error) {
	result := &RollbackEntityMergeResult{
		MergeID: mergeID, CompensatedClaimIDs: []uuid.UUID{},
	}
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.checkMergeActor(ctx, tx, actorID); err != nil {
			return err
		}
		merge, err := s.repo.getEntityMergeForUpdate(ctx, tx, mergeID)
		if err != nil {
			return err
		}
		result.RestoredEntityID = merge.SourceEntityID
		if merge.Status == EntityMergeRolledBack {
			result.Idempotent = true
			return nil
		}
		if err := s.repo.restoreMergedEntity(ctx, tx, merge.SourceEntityID, merge.TargetEntityID); err != nil {
			return err
		}
		claimMaps, err := s.repo.listMergeClaimsForUpdate(ctx, tx, mergeID)
		if err != nil {
			return err
		}
		for _, mapping := range claimMaps {
			oldClaim, err := s.repo.GetClaimByID(ctx, tx, mapping.OldClaimID)
			if err != nil {
				return err
			}
			newClaim, err := s.repo.GetClaimByID(ctx, tx, mapping.NewClaimID)
			if err != nil {
				return err
			}
			if oldClaim.Status != mergeOldClaimStatus(mapping.OldStatus) ||
				newClaim.Status != mapping.NewStatus || newClaim.SupersededBy != nil {
				return ErrEntityMergeStale
			}
			if err := s.repo.updateClaimMergeStatus(ctx, tx, newClaim.ID,
				mapping.NewStatus, mergeOldClaimStatus(mapping.NewStatus)); err != nil {
				return err
			}
			if err := s.repo.updateClaimMergeStatus(ctx, tx, oldClaim.ID,
				oldClaim.Status, mapping.OldStatus); err != nil {
				return err
			}
			if err := s.emitClaimChanged(ctx, tx, newClaim.ID, newClaim.SubjectEntityID, &oldClaim.ID); err != nil {
				return err
			}
			if err := s.emitClaimChanged(ctx, tx, oldClaim.ID, oldClaim.SubjectEntityID, nil); err != nil {
				return err
			}
			result.CompensatedClaimIDs = append(result.CompensatedClaimIDs, newClaim.ID)
		}
		labelMaps, err := s.repo.listMergeLabels(ctx, tx, mergeID)
		if err != nil {
			return err
		}
		for _, mapping := range labelMaps {
			if mapping.Action == MergeLabelExisting {
				continue
			}
			label, err := s.repo.GetLabel(ctx, tx, merge.TargetEntityID,
				mapping.Language, mapping.TargetLabel)
			if err != nil || label.IsPrimary != mapping.TargetIsPrimary {
				return ErrEntityMergeStale
			}
			if err := s.repo.DeleteLabel(ctx, tx, merge.TargetEntityID,
				mapping.Language, mapping.TargetLabel); err != nil {
				return err
			}
			result.RemovedTargetLabels++
		}
		if err := s.repo.markMergeRolledBack(ctx, tx, mergeID, actorID); err != nil {
			return err
		}
		auditID, err := s.ids.New()
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"source_entity_id":           merge.SourceEntityID,
			"target_entity_id":           merge.TargetEntityID,
			"compensated_claim_count":    len(result.CompensatedClaimIDs),
			"removed_target_label_count": result.RemovedTargetLabels,
		})
		return s.repo.insertMergeAudit(ctx, tx, auditID, actorID,
			"entity.merge_rolled_back", mergeID, payload)
	})
	return result, err
}
