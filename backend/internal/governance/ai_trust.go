package governance

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var (
	ErrAITrustProfileNotFound = errors.New("governance: AI 信任档案不存在")
	ErrInvalidAITrustProfile  = errors.New("governance: AI 信任档案参数非法")
)

const (
	AITrustUntrusted = "untrusted"
	AITrustAssisted  = "assisted"
	AITrustTrusted   = "trusted"
)

type AITrustProfile struct {
	WikiID                uuid.UUID  `json:"wiki_id"`
	ActorID               uuid.UUID  `json:"actor_id"`
	ActorType             string     `json:"actor_type"`
	ActorDisplayName      string     `json:"actor_display_name"`
	TrustLevel            string     `json:"trust_level"`
	RequiredSamplePercent int        `json:"required_sample_percent"`
	Configured            bool       `json:"configured"`
	UpdatedBy             *uuid.UUID `json:"updated_by"`
	CreatedAt             *time.Time `json:"created_at"`
	UpdatedAt             *time.Time `json:"updated_at"`
}

type UpdateAITrustProfileParams struct {
	WikiID                uuid.UUID
	ActorID               uuid.UUID
	TrustLevel            string
	RequiredSamplePercent int
	UpdatedBy             uuid.UUID
}

type AITrustService struct {
	repo *Repository
	auth *AuthorizationService
	txm  *db.TxManager
	ids  *id.Generator
}

func NewAITrustService(
	repo *Repository,
	auth *AuthorizationService,
	txm *db.TxManager,
	ids *id.Generator,
) *AITrustService {
	return &AITrustService{repo: repo, auth: auth, txm: txm, ids: ids}
}

// List returns every active AI/import actor, including a conservative virtual
// default when an administrator has not configured a profile yet.
func (s *AITrustService) List(
	ctx context.Context,
	wikiID, actorID uuid.UUID,
) ([]AITrustProfile, error) {
	if err := s.auth.Check(
		ctx, actorID, wikiID, ActionManage, nil,
	); err != nil {
		return nil, err
	}
	rows, err := s.repo.pool.Query(ctx, `SELECT
		a.id,a.actor_type,a.display_name,
		COALESCE(p.trust_level,'untrusted'),
		COALESCE(p.required_sample_percent,100),
		(p.actor_id IS NOT NULL),
		p.updated_by,p.created_at,p.updated_at
		FROM actor a
		LEFT JOIN ai_trust_profile p
		  ON p.actor_id=a.id AND p.wiki_id=$1
		WHERE a.actor_type IN ('ai','import')
		  AND (a.status='active' OR p.actor_id IS NOT NULL)
		ORDER BY (p.actor_id IS NULL),a.display_name,a.id`, wikiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AITrustProfile{}
	for rows.Next() {
		item := AITrustProfile{WikiID: wikiID}
		if err := rows.Scan(
			&item.ActorID, &item.ActorType, &item.ActorDisplayName,
			&item.TrustLevel, &item.RequiredSamplePercent,
			&item.Configured, &item.UpdatedBy, &item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AITrustService) Update(
	ctx context.Context,
	params UpdateAITrustProfileParams,
) (*AITrustProfile, error) {
	params.TrustLevel = strings.TrimSpace(params.TrustLevel)
	if params.WikiID == uuid.Nil || params.ActorID == uuid.Nil ||
		params.UpdatedBy == uuid.Nil ||
		!validAITrustLevel(params.TrustLevel) ||
		params.RequiredSamplePercent < 0 ||
		params.RequiredSamplePercent > 100 {
		return nil, ErrInvalidAITrustProfile
	}

	var result *AITrustProfile
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(
			ctx, tx, params.UpdatedBy, params.WikiID, ActionManage, nil,
		); err != nil {
			return err
		}
		var actorType, displayName, status string
		if err := tx.QueryRow(ctx, `SELECT actor_type,display_name,status
			FROM actor WHERE id=$1 FOR UPDATE`,
			params.ActorID,
		).Scan(&actorType, &displayName, &status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidActor
			}
			return err
		}
		if status != "active" ||
			(actorType != "ai" && actorType != "import") {
			return fmt.Errorf(
				"%w: actor_type=%s status=%s",
				ErrInvalidAITrustProfile, actorType, status,
			)
		}

		var oldLevel *string
		var oldSample *int
		_ = tx.QueryRow(ctx, `SELECT trust_level,required_sample_percent
			FROM ai_trust_profile
			WHERE wiki_id=$1 AND actor_id=$2`,
			params.WikiID, params.ActorID,
		).Scan(&oldLevel, &oldSample)

		item := &AITrustProfile{
			WikiID: params.WikiID, ActorID: params.ActorID,
			ActorType: actorType, ActorDisplayName: displayName,
			TrustLevel:            params.TrustLevel,
			RequiredSamplePercent: params.RequiredSamplePercent,
			Configured:            true, UpdatedBy: &params.UpdatedBy,
		}
		if err := tx.QueryRow(ctx, `INSERT INTO ai_trust_profile
			(wiki_id,actor_id,trust_level,required_sample_percent,updated_by)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (wiki_id,actor_id) DO UPDATE SET
				trust_level=EXCLUDED.trust_level,
				required_sample_percent=EXCLUDED.required_sample_percent,
				updated_by=EXCLUDED.updated_by,
				updated_at=now()
			RETURNING created_at,updated_at`,
			item.WikiID, item.ActorID, item.TrustLevel,
			item.RequiredSamplePercent, params.UpdatedBy,
		).Scan(&item.CreatedAt, &item.UpdatedAt); err != nil {
			return err
		}
		auditID, err := s.ids.New()
		if err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{
			"actor_id":                    item.ActorID,
			"actor_type":                  item.ActorType,
			"old_trust_level":             oldLevel,
			"old_required_sample_percent": oldSample,
			"trust_level":                 item.TrustLevel,
			"required_sample_percent":     item.RequiredSamplePercent,
		})
		if err != nil {
			return err
		}
		if err := s.repo.InsertAuditWithoutBatch(
			ctx, tx, auditID, params.UpdatedBy,
			"ai_trust.updated", "ai_trust", item.ActorID, payload,
		); err != nil {
			return err
		}
		result = item
		return nil
	})
	return result, err
}

// ApplyPolicy makes AI/import trust an explicit, persisted input to submission
// policy. High-risk decisions remain review-only regardless of trust.
func (s *AITrustService) ApplyPolicy(
	ctx context.Context,
	tx pgx.Tx,
	proposal *Proposal,
	decision *RiskDecision,
) error {
	if proposal == nil || decision == nil {
		return ErrInvalidAITrustProfile
	}
	var actorType string
	if err := s.repo.q(tx).QueryRow(ctx,
		`SELECT actor_type FROM actor WHERE id=$1`,
		proposal.CreatedBy,
	).Scan(&actorType); err != nil {
		return err
	}
	if actorType != "ai" && actorType != "import" {
		return nil
	}
	wikiID := wikiIDForProposal(ctx, tx, s.repo, proposal)
	level, sample := AITrustUntrusted, 100
	err := s.repo.q(tx).QueryRow(ctx, `SELECT
		trust_level,required_sample_percent
		FROM ai_trust_profile
		WHERE wiki_id=$1 AND actor_id=$2`,
		wikiID, proposal.CreatedBy,
	).Scan(&level, &sample)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	decision.AITrustLevel = level
	decision.RequiredSamplePercent = sample
	if level == AITrustUntrusted {
		decision.AutoApprove = false
		decision.SampledForReview = true
		decision.Reasons = appendUnique(
			decision.Reasons, "未受信任 AI 的变更必须人工审核",
		)
		return nil
	}
	if decision.AutoApprove && sampledProposal(proposal.ID, sample) {
		decision.AutoApprove = false
		decision.SampledForReview = true
		decision.Reasons = appendUnique(
			decision.Reasons, "AI 信任策略命中人工抽样",
		)
	}
	return nil
}

func validAITrustLevel(level string) bool {
	return level == AITrustUntrusted ||
		level == AITrustAssisted ||
		level == AITrustTrusted
}

func sampledProposal(id uuid.UUID, percent int) bool {
	if percent <= 0 {
		return false
	}
	if percent >= 100 {
		return true
	}
	bucket := int(binary.BigEndian.Uint16(id[:2]) % 100)
	return bucket < percent
}
