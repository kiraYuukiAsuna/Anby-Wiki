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

var (
	ErrInvalidLogin    = errors.New("auth: invalid or expired login")
	ErrUnauthenticated = errors.New("auth: unauthenticated")
)

const loginLifetime = 10 * time.Minute

type idGenerator interface {
	New() (uuid.UUID, error)
}

// Identity is the verified subset of OIDC claims used by the domain.
type Identity struct {
	Issuer      string
	Subject     string
	DisplayName string
}

// LoginAttempt contains browser-bound one-time values for Authorization Code
// with PKCE. BrowserSecret is sent only in an HttpOnly transient cookie.
type LoginAttempt struct {
	State         string
	BrowserSecret string
	Nonce         string
	CodeVerifier  string
	CodeChallenge string
	ExpiresAt     time.Time
}

// Session contains the opaque cookie value. Only its SHA-256 hash is stored.
type Session struct {
	Token     string
	ActorID   uuid.UUID
	ExpiresAt time.Time
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

// BeginLogin persists a short-lived, browser-bound OIDC transaction.
func (s *Service) BeginLogin(ctx context.Context) (LoginAttempt, error) {
	state, err := randomToken(32)
	if err != nil {
		return LoginAttempt{}, err
	}
	browserSecret, err := randomToken(32)
	if err != nil {
		return LoginAttempt{}, err
	}
	nonce, err := randomToken(32)
	if err != nil {
		return LoginAttempt{}, err
	}
	verifier, err := randomToken(48)
	if err != nil {
		return LoginAttempt{}, err
	}
	id, err := s.ids.New()
	if err != nil {
		return LoginAttempt{}, fmt.Errorf("auth: generate login id: %w", err)
	}
	expiresAt := s.now().Add(loginLifetime)
	stateHash := tokenHash(state)
	browserHash := tokenHash(browserSecret)
	if _, err := s.pool.Exec(ctx, `INSERT INTO oidc_login_attempt
		(id,state_hash,browser_secret_hash,nonce,code_verifier,expires_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		id, stateHash[:], browserHash[:], nonce, verifier, expiresAt); err != nil {
		return LoginAttempt{}, fmt.Errorf("auth: persist login attempt: %w", err)
	}
	challengeHash := sha256.Sum256([]byte(verifier))
	return LoginAttempt{
		State:         state,
		BrowserSecret: browserSecret,
		Nonce:         nonce,
		CodeVerifier:  verifier,
		CodeChallenge: base64.RawURLEncoding.EncodeToString(challengeHash[:]),
		ExpiresAt:     expiresAt,
	}, nil
}

// ConsumeLogin atomically consumes a login transaction before token exchange.
func (s *Service) ConsumeLogin(ctx context.Context, state, browserSecret string) (LoginAttempt, error) {
	if state == "" || browserSecret == "" {
		return LoginAttempt{}, ErrInvalidLogin
	}
	stateHash := tokenHash(state)
	browserHash := tokenHash(browserSecret)
	var attempt LoginAttempt
	err := s.pool.QueryRow(ctx, `DELETE FROM oidc_login_attempt
		WHERE state_hash=$1 AND browser_secret_hash=$2 AND expires_at>now()
		RETURNING nonce,code_verifier,expires_at`,
		stateHash[:], browserHash[:]).Scan(&attempt.Nonce, &attempt.CodeVerifier, &attempt.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginAttempt{}, ErrInvalidLogin
	}
	if err != nil {
		return LoginAttempt{}, fmt.Errorf("auth: consume login attempt: %w", err)
	}
	return attempt, nil
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
		identity.DisplayName = "OIDC user"
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
		err := tx.QueryRow(ctx, `SELECT ei.actor_id,a.status FROM external_identity ei
			JOIN actor a ON a.id=ei.actor_id
			WHERE ei.issuer=$1 AND ei.subject=$2`,
			identity.Issuer, identity.Subject).Scan(&session.ActorID, &actorStatus)
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
