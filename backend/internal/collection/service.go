package collection

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

const (
	DefaultPageSize  = 20
	MaxPageSize      = 100
	MaxManualMembers = 10000
)

type Service struct {
	repo   *Repository
	actors *page.Repository
	txm    *db.TxManager
	ids    *id.Generator
}

func NewService(
	repo *Repository, actors *page.Repository, txm *db.TxManager, ids *id.Generator,
) *Service {
	return &Service{repo: repo, actors: actors, txm: txm, ids: ids}
}

func (s *Service) Create(ctx context.Context, params CreateParams) (*Collection, error) {
	title := strings.TrimSpace(params.Title)
	if title == "" || (params.CollectionType != TypeManual &&
		params.CollectionType != TypeRule && params.CollectionType != TypeDynamic) {
		return nil, ErrInvalidDefinition
	}
	var rule Rule
	var query json.RawMessage
	var err error
	if params.CollectionType == TypeManual {
		if len(params.Rule) != 0 && string(params.Rule) != "null" {
			return nil, ErrInvalidDefinition
		}
	} else if params.CollectionType == TypeRule {
		rule, err = ParseRule(params.Rule)
		if err != nil {
			return nil, err
		}
		query, err = json.Marshal(rule)
		if err != nil {
			return nil, err
		}
	} else {
		dynamic, parseErr := ParseDynamicQuery(params.Rule)
		if parseErr != nil {
			return nil, parseErr
		}
		query, err = json.Marshal(dynamic)
		if err != nil {
			return nil, err
		}
	}
	var value *Collection
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.actors.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
			return err
		}
		if params.DescriptionPageID != nil {
			if err := s.repo.ValidatePage(ctx, tx, params.WikiID, *params.DescriptionPageID); err != nil {
				return err
			}
		}
		if params.CollectionType == TypeRule {
			if err := s.repo.ValidateRuleReference(ctx, tx, rule); err != nil {
				return err
			}
		} else if params.CollectionType == TypeDynamic {
			dynamic, err := ParseDynamicQuery(query)
			if err != nil {
				return err
			}
			if err := s.repo.ValidateDynamicReferences(
				ctx, tx, params.WikiID, dynamic,
			); err != nil {
				return err
			}
		}
		collectionID, err := s.ids.New()
		if err != nil {
			return err
		}
		value = &Collection{
			ID: collectionID, WikiID: params.WikiID,
			CollectionType: params.CollectionType, Title: title,
			DescriptionPageID: params.DescriptionPageID, QueryJSON: query,
			CreatedBy: params.ActorID,
		}
		return s.repo.Insert(ctx, tx, value)
	})
	return value, err
}

func (s *Service) ReplaceManualMembers(
	ctx context.Context, wikiID, collectionID, actorID uuid.UUID,
	inputs []MemberInput,
) error {
	if len(inputs) > MaxManualMembers {
		return fmt.Errorf("%w: at most %d members are allowed", ErrInvalidMember, MaxManualMembers)
	}
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.actors.CheckWriteActor(ctx, tx, actorID); err != nil {
			return err
		}
		value, err := s.repo.Lock(ctx, tx, collectionID)
		if err != nil {
			return err
		}
		if value.WikiID != wikiID {
			return ErrNotFound
		}
		if value.CollectionType != TypeManual {
			return ErrInvalidDefinition
		}
		oldPageIDs, oldEntityIDs, err := s.repo.MemberTargetIDs(
			ctx, tx, collectionID,
		)
		if err != nil {
			return err
		}
		members := make([]Membership, 0, len(inputs))
		seen := make(map[string]struct{}, len(inputs))
		for _, input := range inputs {
			member, err := s.validateManualMember(ctx, tx, value.WikiID, input)
			if err != nil {
				return err
			}
			target := memberTarget(member)
			key := member.MemberType + ":" + target.String()
			if _, exists := seen[key]; exists {
				return fmt.Errorf("%w: duplicate target", ErrInvalidMember)
			}
			seen[key] = struct{}{}
			members = append(members, member)
		}
		if err := s.repo.ReplaceMembers(ctx, tx, collectionID, members); err != nil {
			return err
		}
		if err := s.repo.Touch(ctx, tx, collectionID); err != nil {
			return err
		}
		newPageIDs, newEntityIDs := membershipTargetIDs(members)
		return s.appendMembershipReplacement(
			ctx, tx, collectionID, actorID,
			append(oldPageIDs, newPageIDs...),
			append(oldEntityIDs, newEntityIDs...),
		)
	})
}

func membershipTargetIDs(members []Membership) ([]uuid.UUID, []uuid.UUID) {
	pageIDs := make([]uuid.UUID, 0, len(members))
	entityIDs := make([]uuid.UUID, 0, len(members))
	for _, member := range members {
		if member.PageID != nil {
			pageIDs = append(pageIDs, *member.PageID)
		}
		if member.EntityID != nil {
			entityIDs = append(entityIDs, *member.EntityID)
		}
	}
	return pageIDs, entityIDs
}

func (s *Service) appendMembershipReplacement(
	ctx context.Context,
	tx pgx.Tx,
	collectionID, actorID uuid.UUID,
	pageIDs, entityIDs []uuid.UUID,
) error {
	pageIDs = uniqueSortedIDs(pageIDs)
	entityIDs = uniqueSortedIDs(entityIDs)
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	outboxID, err := s.ids.New()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"collection_id": collectionID,
		"page_ids":      pageIDs,
		"entity_ids":    entityIDs,
	})
	if err != nil {
		return err
	}
	return s.repo.InsertMembershipEvent(
		ctx, tx, auditID, outboxID, collectionID, actorID, nil,
		OutboxEventMembershipReplaced, payload,
	)
}

func uniqueSortedIDs(values []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(values))
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if value == uuid.Nil {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].String() < result[j].String()
	})
	return result
}

// ApplyManualMemberInTx is the authoritative single-member boundary used by
// Proposal Apply. add=false removes the exact target; both paths are audited
// and emit an Outbox event in the caller's ChangeBatch transaction.
func (s *Service) ApplyManualMemberInTx(
	ctx context.Context,
	tx pgx.Tx,
	wikiID, collectionID, actorID uuid.UUID,
	input MemberInput,
	add bool,
	changeBatchID *uuid.UUID,
) (Membership, error) {
	if err := s.actors.CheckWriteActor(ctx, tx, actorID); err != nil {
		return Membership{}, err
	}
	value, err := s.repo.Lock(ctx, tx, collectionID)
	if err != nil {
		return Membership{}, err
	}
	if value.WikiID != wikiID {
		return Membership{}, ErrNotFound
	}
	if value.CollectionType != TypeManual {
		return Membership{}, ErrInvalidDefinition
	}
	var member Membership
	if add {
		member, err = s.validateManualMember(ctx, tx, value.WikiID, input)
		if err != nil {
			return Membership{}, err
		}
		member.CollectionID = collectionID
		if err := s.repo.AddMember(ctx, tx, collectionID, member); err != nil {
			return Membership{}, err
		}
	} else {
		member = Membership{
			PageID: input.PageID, EntityID: input.EntityID,
			MemberType: input.MemberType, SourceType: TypeManual,
		}
		switch input.MemberType {
		case MemberPage:
			if input.PageID == nil || input.EntityID != nil {
				return Membership{}, ErrInvalidMember
			}
		case MemberEntity:
			if input.EntityID == nil || input.PageID != nil {
				return Membership{}, ErrInvalidMember
			}
		default:
			return Membership{}, ErrInvalidMember
		}
		member, err = s.repo.GetMemberForUpdate(
			ctx, tx, collectionID, member.MemberType, member.PageID, member.EntityID,
		)
		if err != nil {
			return Membership{}, err
		}
		if err := s.repo.RemoveMember(
			ctx, tx, collectionID, member.MemberType, member.PageID, member.EntityID,
		); err != nil {
			return Membership{}, err
		}
	}
	if err := s.repo.Touch(ctx, tx, collectionID); err != nil {
		return Membership{}, err
	}
	auditID, err := s.ids.New()
	if err != nil {
		return Membership{}, err
	}
	outboxID, err := s.ids.New()
	if err != nil {
		return Membership{}, err
	}
	eventType := OutboxEventMembershipRemoved
	if add {
		eventType = OutboxEventMembershipAdded
	}
	payload, _ := json.Marshal(map[string]any{
		"collection_id":      collectionID,
		"member_type":        member.MemberType,
		"page_id":            member.PageID,
		"entity_id":          member.EntityID,
		"sort_key":           member.SortKey,
		"source_type":        member.SourceType,
		"source_revision_id": member.SourceRevisionID,
		"change_batch_id":    changeBatchID,
	})
	if err := s.repo.InsertMembershipEvent(
		ctx, tx, auditID, outboxID, collectionID, actorID,
		changeBatchID, eventType, payload,
	); err != nil {
		return Membership{}, err
	}
	return member, nil
}

func (s *Service) validateManualMember(
	ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, input MemberInput,
) (Membership, error) {
	sortKey := strings.TrimSpace(input.SortKey)
	if sortKey == "" {
		return Membership{}, ErrInvalidMember
	}
	if err := s.repo.ValidateRevision(ctx, tx, wikiID, input.SourceRevisionID); err != nil {
		return Membership{}, err
	}
	member := Membership{
		PageID: input.PageID, EntityID: input.EntityID, MemberType: input.MemberType,
		SourceType: TypeManual, SortKey: sortKey,
		SourceRevisionID: &input.SourceRevisionID,
	}
	switch input.MemberType {
	case MemberPage:
		if input.PageID == nil || input.EntityID != nil {
			return Membership{}, ErrInvalidMember
		}
		if err := s.repo.ValidatePage(ctx, tx, wikiID, *input.PageID); err != nil {
			return Membership{}, err
		}
	case MemberEntity:
		if input.EntityID == nil || input.PageID != nil {
			return Membership{}, ErrInvalidMember
		}
		if err := s.repo.ValidateEntity(ctx, tx, wikiID, *input.EntityID); err != nil {
			return Membership{}, err
		}
	default:
		return Membership{}, ErrInvalidMember
	}
	return member, nil
}

func (s *Service) RebuildRule(
	ctx context.Context, wikiID, collectionID, sourceRevisionID, actorID uuid.UUID,
) (int, error) {
	count := 0
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.actors.CheckWriteActor(ctx, tx, actorID); err != nil {
			return err
		}
		value, err := s.repo.Lock(ctx, tx, collectionID)
		if err != nil {
			return err
		}
		if value.WikiID != wikiID {
			return ErrNotFound
		}
		if value.CollectionType != TypeRule {
			return ErrInvalidDefinition
		}
		oldPageIDs, oldEntityIDs, err := s.repo.MemberTargetIDs(
			ctx, tx, collectionID,
		)
		if err != nil {
			return err
		}
		if err := s.repo.ValidateRevision(ctx, tx, value.WikiID, sourceRevisionID); err != nil {
			return err
		}
		rule, err := ParseRule(value.QueryJSON)
		if err != nil {
			return err
		}
		if err := s.repo.ValidateRuleReference(ctx, tx, rule); err != nil {
			return err
		}
		entityIDs, err := s.repo.ResolveRule(ctx, tx, value.WikiID, rule)
		if err != nil {
			return err
		}
		members := make([]Membership, 0, len(entityIDs))
		for _, entityID := range entityIDs {
			id := entityID
			members = append(members, Membership{
				EntityID: &id, MemberType: MemberEntity, SourceType: TypeRule,
				SortKey: entityID.String(), SourceRevisionID: &sourceRevisionID,
			})
		}
		count = len(members)
		if err := s.repo.ReplaceMembers(ctx, tx, collectionID, members); err != nil {
			return err
		}
		if err := s.repo.Touch(ctx, tx, collectionID); err != nil {
			return err
		}
		newPageIDs, newEntityIDs := membershipTargetIDs(members)
		return s.appendMembershipReplacement(
			ctx, tx, collectionID, actorID,
			append(oldPageIDs, newPageIDs...),
			append(oldEntityIDs, newEntityIDs...),
		)
	})
	return count, err
}

func memberTarget(member Membership) uuid.UUID {
	if member.PageID != nil {
		return *member.PageID
	}
	return *member.EntityID
}

func (s *Service) Get(
	ctx context.Context, wikiID, id uuid.UUID,
) (*Collection, error) {
	value, err := s.repo.Get(ctx, nil, id)
	if err != nil {
		return nil, err
	}
	if value.WikiID != wikiID {
		return nil, ErrNotFound
	}
	return value, nil
}

func (s *Service) List(
	ctx context.Context, wikiID uuid.UUID, cursor string, limit int,
) (*CollectionPage, error) {
	return s.repo.List(ctx, wikiID, cursor, normalizeLimit(limit))
}

func (s *Service) ListForPage(
	ctx context.Context,
	wikiID, pageID uuid.UUID,
) ([]PageCollection, error) {
	value, err := s.actors.GetPageByID(ctx, nil, pageID)
	if err != nil {
		return nil, err
	}
	if value.WikiID != wikiID || value.DeletedAt != nil {
		return nil, page.ErrPageNotFound
	}
	return s.repo.ListForPage(ctx, wikiID, pageID)
}

func (s *Service) ListMembers(
	ctx context.Context, wikiID, collectionID uuid.UUID, cursor string, limit int,
) (*MembershipPage, error) {
	value, err := s.Get(ctx, wikiID, collectionID)
	if err != nil {
		return nil, err
	}
	if value.CollectionType == TypeDynamic {
		query, err := ParseDynamicQuery(value.QueryJSON)
		if err != nil {
			return nil, err
		}
		return s.repo.ListDynamicMembers(
			ctx, collectionID, wikiID, query, cursor, normalizeLimit(limit),
		)
	}
	return s.repo.ListMembers(ctx, collectionID, cursor, normalizeLimit(limit))
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultPageSize
	}
	if limit > MaxPageSize {
		return MaxPageSize
	}
	return limit
}
