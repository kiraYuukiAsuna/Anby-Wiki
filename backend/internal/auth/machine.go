package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
)

const (
	ActorTypeHuman        = "human"
	ActorTypeAI           = "ai"
	ActorTypeImport       = "import"
	ActorStatusActive     = "active"
	ActorStatusDisabled   = "disabled"
	MachineActorKeyPrefix = "managed-machine:"
	machineCreatedEvent   = "machine_actor.created"
	machineStatusEvent    = "machine_actor.status_changed"
)

var (
	ErrMachineActorForbidden = errors.New("auth: machine Actor requires an active human administrator")
	ErrMachineActorNotFound  = errors.New("auth: machine Actor not found")
	ErrMachineActorConflict  = errors.New("auth: machine Actor idempotency mismatch")
	ErrInvalidMachineActor   = errors.New("auth: invalid machine Actor input")
)

// CLIActorIdentity always identifies the credential's Actor, never its owner.
type CLIActorIdentity struct {
	ActorID     uuid.UUID `json:"actor_id"`
	WikiID      uuid.UUID `json:"wiki_id"`
	ActorType   string    `json:"actor_type"`
	DisplayName string    `json:"display_name"`
}

type MachineActor struct {
	CLIActorIdentity
	OwnerActorID uuid.UUID `json:"owner_actor_id"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

type CreateMachineActorParams struct {
	WikiID         uuid.UUID
	CreatedBy      uuid.UUID
	ActorType      string
	DisplayName    string
	IdempotencyKey string
}

type SetMachineActorStatusParams struct {
	WikiID    uuid.UUID
	ActorID   uuid.UUID
	UpdatedBy uuid.UUID
	Status    string
}

func (s *Service) CreateMachineActor(ctx context.Context, in CreateMachineActorParams) (MachineActor, error) {
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if in.WikiID == uuid.Nil || in.CreatedBy == uuid.Nil ||
		(in.ActorType != ActorTypeAI && in.ActorType != ActorTypeImport) ||
		in.DisplayName == "" || !utf8.ValidString(in.DisplayName) ||
		utf8.RuneCountInString(in.DisplayName) > 120 ||
		in.IdempotencyKey == "" || len(in.IdempotencyKey) > 200 {
		return MachineActor{}, ErrInvalidMachineActor
	}
	var result MachineActor
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		// Serialize Wiki-local creation keys without a second identity schema.
		var wikiID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM wiki_site WHERE id=$1 FOR UPDATE`, in.WikiID).Scan(&wikiID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrMachineActorForbidden
			}
			return err
		}
		accountID, err := machineAdministrator(ctx, tx, in.CreatedBy, in.WikiID)
		if err != nil {
			return err
		}
		key := MachineActorKeyPrefix + in.IdempotencyKey
		err = tx.QueryRow(ctx, `SELECT a.id,a.wiki_id,a.actor_type,a.display_name,
			COALESCE(la.actor_id,$3),a.status,a.created_at
			FROM actor a LEFT JOIN local_account la ON la.id=a.user_id
			WHERE a.wiki_id=$1 AND a.external_key=$2`, in.WikiID, key, uuid.Nil).
			Scan(&result.ActorID, &result.WikiID, &result.ActorType, &result.DisplayName,
				&result.OwnerActorID, &result.Status, &result.CreatedAt)
		if err == nil {
			if result.OwnerActorID != in.CreatedBy || result.ActorType != in.ActorType || result.DisplayName != in.DisplayName {
				return ErrMachineActorConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		actorID, err := s.ids.New()
		if err != nil {
			return err
		}
		result = MachineActor{
			CLIActorIdentity: CLIActorIdentity{ActorID: actorID, WikiID: in.WikiID, ActorType: in.ActorType, DisplayName: in.DisplayName},
			OwnerActorID:     in.CreatedBy, Status: ActorStatusActive,
		}
		if err := tx.QueryRow(ctx, `INSERT INTO actor
			(id,wiki_id,user_id,actor_type,display_name,external_key,status)
			VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`,
			actorID, in.WikiID, accountID, in.ActorType, in.DisplayName, key, ActorStatusActive).Scan(&result.CreatedAt); err != nil {
			return err
		}
		return s.auditMachineActor(ctx, tx, in.CreatedBy, machineCreatedEvent, result, "")
	})
	return result, err
}

func machineAdministrator(ctx context.Context, q db.Querier, actorID, wikiID uuid.UUID) (uuid.UUID, error) {
	var accountID uuid.UUID
	err := q.QueryRow(ctx, `SELECT la.id FROM local_account la
		JOIN actor a ON a.id=la.actor_id
		WHERE a.id=$1 AND a.actor_type='human' AND a.status='active' AND la.status='active'
		  AND EXISTS (SELECT 1 FROM actor_role ar JOIN role r ON r.id=ar.role_id
		    WHERE ar.actor_id=a.id AND ar.wiki_id=$2 AND r.role_key='admin')`, actorID, wikiID).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrMachineActorForbidden
	}
	return accountID, err
}

// ListMachineActors is Wiki administration; CLI candidates are account-owned
// and retain no permission when their owner loses administrator access.
func (s *Service) ListMachineActors(ctx context.Context, actorID, wikiID uuid.UUID) ([]MachineActor, error) {
	if _, err := machineAdministrator(ctx, s.pool, actorID, wikiID); err != nil {
		return nil, err
	}
	return s.listMachineActors(ctx, actorID, wikiID, false)
}

func (s *Service) ListCLIMachineActors(ctx context.Context, actorID uuid.UUID) ([]MachineActor, error) {
	return s.listMachineActors(ctx, actorID, uuid.Nil, true)
}

func (s *Service) listMachineActors(ctx context.Context, actorID, wikiID uuid.UUID, ownedOnly bool) ([]MachineActor, error) {
	rows, err := s.pool.Query(ctx, `SELECT a.id,a.wiki_id,a.actor_type,a.display_name,
		COALESCE(la.actor_id,$5),a.status,a.created_at
		FROM actor a LEFT JOIN local_account la ON la.id=a.user_id
		LEFT JOIN actor owner ON owner.id=la.actor_id
		WHERE a.actor_type IN ('ai','import') AND starts_with(a.external_key,$1)
		  AND (($4 AND la.actor_id=$2 AND la.status='active' AND owner.status='active'
		    AND EXISTS (SELECT 1 FROM actor_role ar JOIN role r ON r.id=ar.role_id
		      WHERE ar.actor_id=owner.id AND ar.wiki_id=a.wiki_id AND r.role_key='admin'))
		    OR (NOT $4 AND a.wiki_id=$3))
		ORDER BY a.created_at,a.id`, MachineActorKeyPrefix, actorID, wikiID, ownedOnly, uuid.Nil)
	if err != nil {
		return nil, fmt.Errorf("auth: list machine Actors: %w", err)
	}
	defer rows.Close()
	result := []MachineActor{}
	for rows.Next() {
		var item MachineActor
		if err := rows.Scan(&item.ActorID, &item.WikiID, &item.ActorType, &item.DisplayName,
			&item.OwnerActorID, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Service) SetMachineActorStatus(ctx context.Context, in SetMachineActorStatusParams) (MachineActor, error) {
	if in.Status != ActorStatusActive && in.Status != ActorStatusDisabled {
		return MachineActor{}, ErrInvalidMachineActor
	}
	var result MachineActor
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := machineAdministrator(ctx, tx, in.UpdatedBy, in.WikiID); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT a.id,a.wiki_id,a.actor_type,a.display_name,
			COALESCE(la.actor_id,$4),a.status,a.created_at
			FROM actor a LEFT JOIN local_account la ON la.id=a.user_id
			WHERE a.id=$1 AND a.wiki_id=$2 AND a.actor_type IN ('ai','import')
			  AND starts_with(a.external_key,$3) FOR UPDATE OF a`, in.ActorID, in.WikiID, MachineActorKeyPrefix, uuid.Nil).
			Scan(&result.ActorID, &result.WikiID, &result.ActorType, &result.DisplayName,
				&result.OwnerActorID, &result.Status, &result.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMachineActorNotFound
		}
		if err != nil {
			return err
		}
		if in.Status == ActorStatusActive && result.OwnerActorID == uuid.Nil {
			return ErrMachineActorForbidden
		}
		oldStatus := result.Status
		if oldStatus == in.Status {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE actor SET status=$2 WHERE id=$1`, in.ActorID, in.Status); err != nil {
			return err
		}
		// Re-enabling is a fresh authorization boundary; old credentials never
		// become valid again. The Actor row lock also serializes code exchange.
		if _, err := tx.Exec(ctx, `UPDATE cli_access_token SET revoked_at=COALESCE(revoked_at,now()) WHERE actor_id=$1`, in.ActorID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM cli_authorization_code WHERE actor_id=$1`, in.ActorID); err != nil {
			return err
		}
		result.Status = in.Status
		return s.auditMachineActor(ctx, tx, in.UpdatedBy, machineStatusEvent, result, oldStatus)
	})
	return result, err
}

func (s *Service) auditMachineActor(ctx context.Context, tx pgx.Tx, by uuid.UUID, event string, actor MachineActor, oldStatus string) error {
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"wiki_id": actor.WikiID, "actor_type": actor.ActorType, "owner_actor_id": actor.OwnerActorID,
		"old_status": oldStatus, "status": actor.Status,
	})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_event (id,actor_id,event_type,aggregate_type,aggregate_id,payload_json)
		VALUES ($1,$2,$3,'actor',$4,$5)`, auditID, by, event, actor.ActorID, payload)
	return err
}

// lockCLIActor is shared by issuance and exchange, so disabling cannot race
// with minting. A machine requires its active owning human to remain an admin.
func lockCLIActor(ctx context.Context, tx pgx.Tx, actorID, issuedBy uuid.UUID) (CLIActorIdentity, error) {
	var result CLIActorIdentity
	var ownerID *uuid.UUID
	var status string
	var eligible bool
	err := tx.QueryRow(ctx, `SELECT a.id,COALESCE(a.wiki_id,$3),a.actor_type,a.display_name,
		la.actor_id,a.status,
		COALESCE(CASE WHEN a.actor_type='human' THEN la.id IS NULL OR la.status='active'
		  ELSE starts_with(a.external_key,$2) AND la.status='active' AND owner.status='active'
		    AND owner.actor_type='human' AND EXISTS (
		      SELECT 1 FROM actor_role ar JOIN role r ON r.id=ar.role_id
		      WHERE ar.actor_id=owner.id AND ar.wiki_id=a.wiki_id AND r.role_key='admin')
		END,false)
		FROM actor a LEFT JOIN local_account la ON la.id=a.user_id
		LEFT JOIN actor owner ON owner.id=la.actor_id
		WHERE a.id=$1 FOR UPDATE OF a`, actorID, MachineActorKeyPrefix, uuid.Nil).
		Scan(&result.ActorID, &result.WikiID, &result.ActorType, &result.DisplayName, &ownerID, &status, &eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		return CLIActorIdentity{}, ErrUnauthenticated
	}
	if err != nil {
		return CLIActorIdentity{}, err
	}
	if status != ActorStatusActive || !eligible ||
		(result.ActorType != ActorTypeHuman && result.ActorType != ActorTypeAI && result.ActorType != ActorTypeImport) {
		return CLIActorIdentity{}, ErrUnauthenticated
	}
	if issuedBy != uuid.Nil {
		if result.ActorType == ActorTypeHuman && actorID != issuedBy {
			return CLIActorIdentity{}, ErrMachineActorForbidden
		}
		if result.ActorType != ActorTypeHuman && (ownerID == nil || *ownerID != issuedBy) {
			return CLIActorIdentity{}, ErrMachineActorForbidden
		}
	}
	return result, nil
}
