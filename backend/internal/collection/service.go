package collection

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
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
	if title == "" || (params.CollectionType != TypeManual && params.CollectionType != TypeRule) {
		return nil, ErrInvalidDefinition
	}
	var rule Rule
	var query json.RawMessage
	var err error
	if params.CollectionType == TypeManual {
		if len(params.Rule) != 0 && string(params.Rule) != "null" {
			return nil, ErrInvalidDefinition
		}
	} else {
		rule, err = ParseRule(params.Rule)
		if err != nil {
			return nil, err
		}
		query, err = json.Marshal(rule)
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
	ctx context.Context, collectionID, actorID uuid.UUID, inputs []MemberInput,
) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.actors.CheckWriteActor(ctx, tx, actorID); err != nil {
			return err
		}
		value, err := s.repo.Lock(ctx, tx, collectionID)
		if err != nil {
			return err
		}
		if value.CollectionType != TypeManual {
			return ErrInvalidDefinition
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
		return s.repo.ReplaceMembers(ctx, tx, collectionID, members)
	})
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
		SourceType: TypeManual, SortKey: sortKey, SourceRevisionID: input.SourceRevisionID,
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
	ctx context.Context, collectionID, sourceRevisionID, actorID uuid.UUID,
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
		if value.CollectionType != TypeRule {
			return ErrInvalidDefinition
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
				SortKey: entityID.String(), SourceRevisionID: sourceRevisionID,
			})
		}
		count = len(members)
		return s.repo.ReplaceMembers(ctx, tx, collectionID, members)
	})
	return count, err
}

func memberTarget(member Membership) uuid.UUID {
	if member.PageID != nil {
		return *member.PageID
	}
	return *member.EntityID
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Collection, error) {
	return s.repo.Get(ctx, nil, id)
}

func (s *Service) List(
	ctx context.Context, wikiID uuid.UUID, cursor string, limit int,
) (*CollectionPage, error) {
	return s.repo.List(ctx, wikiID, cursor, normalizeLimit(limit))
}

func (s *Service) ListMembers(
	ctx context.Context, collectionID uuid.UUID, cursor string, limit int,
) (*MembershipPage, error) {
	if _, err := s.repo.Get(ctx, nil, collectionID); err != nil {
		return nil, err
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
