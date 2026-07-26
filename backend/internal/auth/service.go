package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
)

var ErrUnauthenticated = errors.New("auth: unauthenticated")

// BootstrapIssuer marks identities created by the early-stage bootstrap token
// login. It is not a real OpenID Connect issuer and performs no identity
// verification; see AuthAPI.devLogin.
const BootstrapIssuer = "urn:anby-wiki:bootstrap"

type idGenerator interface {
	New() (uuid.UUID, error)
}

// Identity is the verified external identity used by the domain.
type Identity struct {
	Issuer      string
	Subject     string
	DisplayName string
}

// Session contains the opaque cookie value. Only its SHA-256 hash is stored.
type Session struct {
	Token       string
	ActorID     uuid.UUID
	DisplayName string
	ExpiresAt   time.Time
}

// Service is the sole write path for external identities and auth sessions.
type Service struct {
	pool       db.Querier
	txm        *db.TxManager
	ids        idGenerator
	sessionTTL time.Duration
	now        func() time.Time
}

func NewService(pool db.Querier, txm *db.TxManager, ids idGenerator, sessionTTL time.Duration) *Service {
	return &Service{
		pool:       pool,
		txm:        txm,
		ids:        ids,
		sessionTTL: sessionTTL,
		now:        time.Now,
	}
}

// EstablishSession maps the verified (issuer, subject) to one human Actor and
// creates a new opaque server-side session in the same transaction.
func (s *Service) EstablishSession(ctx context.Context, identity Identity) (Session, error) {
	identity.Issuer = strings.TrimSpace(identity.Issuer)
	identity.Subject = strings.TrimSpace(identity.Subject)
	identity.DisplayName = strings.TrimSpace(identity.DisplayName)
	if identity.Issuer == "" || identity.Subject == "" {
		return Session{}, fmt.Errorf("auth: verified identity lacks issuer or subject")
	}
	if identity.DisplayName == "" {
		identity.DisplayName = "Unnamed user"
	}
	token, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	sessionID, err := s.ids.New()
	if err != nil {
		return Session{}, fmt.Errorf("auth: generate session id: %w", err)
	}
	expiresAt := s.now().Add(s.sessionTTL)
	hash := tokenHash(token)
	session := Session{Token: token, ExpiresAt: expiresAt}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		// Serialize first login for a subject without exposing the subject to logs.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			identity.Issuer+"\x1f"+identity.Subject); err != nil {
			return err
		}
		var actorStatus string
		err := tx.QueryRow(ctx, `SELECT ei.actor_id,a.status,a.display_name FROM external_identity ei
			JOIN actor a ON a.id=ei.actor_id
			WHERE ei.issuer=$1 AND ei.subject=$2`,
			identity.Issuer, identity.Subject).Scan(
			&session.ActorID, &actorStatus, &session.DisplayName,
		)
		switch {
		case err == nil:
			if actorStatus != "active" {
				return ErrUnauthenticated
			}
			if _, err := tx.Exec(ctx, `UPDATE external_identity SET last_seen_at=now()
				WHERE issuer=$1 AND subject=$2`, identity.Issuer, identity.Subject); err != nil {
				return err
			}
		case errors.Is(err, pgx.ErrNoRows):
			session.ActorID, err = s.ids.New()
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO actor
				(id,actor_type,display_name,status) VALUES ($1,'human',$2,'active')`,
				session.ActorID, identity.DisplayName); err != nil {
				return err
			}
			session.DisplayName = identity.DisplayName
			if _, err := tx.Exec(ctx, `INSERT INTO external_identity
				(issuer,subject,actor_id) VALUES ($1,$2,$3)`,
				identity.Issuer, identity.Subject, session.ActorID); err != nil {
				return err
			}
		default:
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO auth_session
			(id,token_hash,actor_id,expires_at) VALUES ($1,$2,$3,$4)`,
			sessionID, hash[:], session.ActorID, expiresAt)
		return err
	})
	if err != nil {
		return Session{}, fmt.Errorf("auth: establish session: %w", err)
	}
	return session, nil
}

// ResolveSession authenticates an opaque token and re-reads Actor status on
// every request. No role or permission is cached in the session.
func (s *Service) ResolveSession(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}
	hash := tokenHash(token)
	var principal Principal
	err := s.pool.QueryRow(ctx, `SELECT a.id,a.actor_type,a.display_name FROM auth_session s
		JOIN actor a ON a.id=s.actor_id
		WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now()
			AND a.status='active'`, hash[:]).Scan(
		&principal.ActorID, &principal.ActorType, &principal.DisplayName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("auth: resolve session: %w", err)
	}
	principal.Method = "session"
	return principal, nil
}

// ResolveDevActor validates an explicitly supplied development/test Actor.
func (s *Service) ResolveDevActor(ctx context.Context, actorID uuid.UUID) (Principal, error) {
	var principal Principal
	err := s.pool.QueryRow(ctx, `SELECT id,actor_type,display_name FROM actor
		WHERE id=$1 AND status='active'`, actorID).Scan(
		&principal.ActorID, &principal.ActorType, &principal.DisplayName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("auth: resolve development actor: %w", err)
	}
	principal.Method = "development_header"
	return principal, nil
}

// RevokeSession immediately invalidates the supplied opaque token.
func (s *Service) RevokeSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	hash := tokenHash(token)
	if _, err := s.pool.Exec(ctx, `UPDATE auth_session SET revoked_at=now()
		WHERE token_hash=$1 AND revoked_at IS NULL`, hash[:]); err != nil {
		return fmt.Errorf("auth: revoke session: %w", err)
	}
	return nil
}

func randomToken(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("auth: generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func tokenHash(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}
