package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidCLIInput             = errors.New("auth: invalid CLI authorization input")
	ErrInvalidCLIAuthorizationCode = errors.New("auth: invalid CLI authorization code")
	ErrCLITokenNotFound            = errors.New("auth: CLI token not found")
)

const (
	cliAuthorizationCodeTTL = 10 * time.Minute
	cliTokenMinTTLDays      = 1
	cliTokenMaxTTLDays      = 365
	cliTokenUsageInterval   = 5 * time.Minute
	cliCodePrefix           = "anby_code_"
	cliTokenPrefix          = "anby_token_"
)

// CLIAuthorizationCode is returned once to an authenticated browser session.
// Only CodeHash is persisted.
type CLIAuthorizationCode struct {
	Code           string
	Name           string
	ExpiresAt      time.Time
	TokenExpiresAt time.Time
}

// CLIToken is the redacted token record exposed to the account that owns it.
type CLIToken struct {
	ID          uuid.UUID
	Name        string
	TokenPrefix string
	Status      string
	ExpiresAt   time.Time
	LastUsedAt  *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}

// CLIExchangeResult carries the plaintext token only at exchange time.
type CLIExchangeResult struct {
	Token       string
	ActorID     uuid.UUID
	DisplayName string
	TokenInfo   CLIToken
}

func (s *Service) CreateCLIAuthorizationCode(
	ctx context.Context,
	actorID uuid.UUID,
	name string,
	tokenTTLDays int,
) (CLIAuthorizationCode, error) {
	name, err := validateCLITokenName(name)
	if err != nil || actorID == uuid.Nil ||
		tokenTTLDays < cliTokenMinTTLDays ||
		tokenTTLDays > cliTokenMaxTTLDays {
		return CLIAuthorizationCode{}, ErrInvalidCLIInput
	}
	raw, err := randomToken(24)
	if err != nil {
		return CLIAuthorizationCode{}, err
	}
	code := cliCodePrefix + raw
	codeHash := tokenHash(code)
	id, err := s.ids.New()
	if err != nil {
		return CLIAuthorizationCode{}, fmt.Errorf("auth: generate CLI authorization id: %w", err)
	}
	now := s.now().UTC()
	result := CLIAuthorizationCode{
		Code:           code,
		Name:           name,
		ExpiresAt:      now.Add(cliAuthorizationCodeTTL),
		TokenExpiresAt: now.Add(time.Duration(tokenTTLDays) * 24 * time.Hour),
	}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM actor a
			LEFT JOIN local_account la ON la.actor_id=a.id
			WHERE a.id=$1 AND a.status='active'
			  AND (la.id IS NULL OR la.status='active')
		)`, actorID).Scan(&active); err != nil {
			return err
		}
		if !active {
			return ErrUnauthenticated
		}
		if _, err := tx.Exec(ctx, `DELETE FROM cli_authorization_code
			WHERE actor_id=$1 AND (used_at IS NOT NULL OR expires_at<=now())`,
			actorID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO cli_authorization_code
			(id,code_hash,actor_id,token_name,token_expires_at,expires_at)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			id, codeHash[:], actorID, name, result.TokenExpiresAt, result.ExpiresAt)
		return err
	})
	if err != nil {
		return CLIAuthorizationCode{}, fmt.Errorf("auth: create CLI authorization code: %w", err)
	}
	return result, nil
}

func (s *Service) ExchangeCLIAuthorizationCode(
	ctx context.Context,
	code string,
) (CLIExchangeResult, error) {
	code = strings.TrimSpace(code)
	if !strings.HasPrefix(code, cliCodePrefix) || len(code) > 256 {
		return CLIExchangeResult{}, ErrInvalidCLIAuthorizationCode
	}
	plaintext, err := randomToken(32)
	if err != nil {
		return CLIExchangeResult{}, err
	}
	token := cliTokenPrefix + plaintext
	tokenHashValue := tokenHash(token)
	tokenID, err := s.ids.New()
	if err != nil {
		return CLIExchangeResult{}, fmt.Errorf("auth: generate CLI token id: %w", err)
	}
	codeHash := tokenHash(code)
	var result CLIExchangeResult
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var codeID uuid.UUID
		var name, actorStatus, accountStatus string
		var expiresAt time.Time
		err := tx.QueryRow(ctx, `SELECT
			c.id,c.actor_id,c.token_name,c.token_expires_at,
			a.display_name,a.status,COALESCE(la.status,'active')
			FROM cli_authorization_code c
			JOIN actor a ON a.id=c.actor_id
			LEFT JOIN local_account la ON la.actor_id=a.id
			WHERE c.code_hash=$1 AND c.used_at IS NULL AND c.expires_at>now()
			FOR UPDATE OF c`,
			codeHash[:],
		).Scan(
			&codeID, &result.ActorID, &name, &expiresAt,
			&result.DisplayName, &actorStatus, &accountStatus,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidCLIAuthorizationCode
		}
		if err != nil {
			return err
		}
		if actorStatus != "active" || accountStatus != "active" {
			return ErrInvalidCLIAuthorizationCode
		}
		now := s.now().UTC()
		if !expiresAt.After(now) {
			return ErrInvalidCLIAuthorizationCode
		}
		prefix := redactedCLITokenPrefix(token)
		var createdAt time.Time
		if err := tx.QueryRow(ctx, `INSERT INTO cli_access_token
			(id,token_hash,token_prefix,actor_id,name,expires_at)
			VALUES ($1,$2,$3,$4,$5,$6)
			RETURNING created_at`,
			tokenID, tokenHashValue[:], prefix, result.ActorID, name, expiresAt,
		).Scan(&createdAt); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE cli_authorization_code
			SET used_at=now() WHERE id=$1 AND used_at IS NULL`, codeID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrInvalidCLIAuthorizationCode
		}
		result.Token = token
		result.TokenInfo = CLIToken{
			ID: tokenID, Name: name, TokenPrefix: prefix,
			Status: "active", ExpiresAt: expiresAt, CreatedAt: createdAt,
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidCLIAuthorizationCode) {
			return CLIExchangeResult{}, err
		}
		return CLIExchangeResult{}, fmt.Errorf("auth: exchange CLI authorization code: %w", err)
	}
	return result, nil
}

func (s *Service) ListCLITokens(
	ctx context.Context,
	actorID uuid.UUID,
) ([]CLIToken, error) {
	if actorID == uuid.Nil {
		return nil, ErrUnauthenticated
	}
	rows, err := s.pool.Query(ctx, `SELECT
		id,name,token_prefix,expires_at,last_used_at,revoked_at,created_at
		FROM cli_access_token
		WHERE actor_id=$1
		ORDER BY created_at DESC,id DESC`, actorID)
	if err != nil {
		return nil, fmt.Errorf("auth: list CLI tokens: %w", err)
	}
	defer rows.Close()
	now := s.now().UTC()
	result := []CLIToken{}
	for rows.Next() {
		var item CLIToken
		if err := rows.Scan(
			&item.ID, &item.Name, &item.TokenPrefix, &item.ExpiresAt,
			&item.LastUsedAt, &item.RevokedAt, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("auth: scan CLI token: %w", err)
		}
		item.Status = cliTokenStatus(item, now)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: iterate CLI tokens: %w", err)
	}
	return result, nil
}

func (s *Service) RevokeCLIToken(
	ctx context.Context,
	actorID, tokenID uuid.UUID,
) error {
	if actorID == uuid.Nil || tokenID == uuid.Nil {
		return ErrCLITokenNotFound
	}
	tag, err := s.pool.Exec(ctx, `UPDATE cli_access_token
		SET revoked_at=COALESCE(revoked_at,now())
		WHERE id=$1 AND actor_id=$2`, tokenID, actorID)
	if err != nil {
		return fmt.Errorf("auth: revoke CLI token: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrCLITokenNotFound
	}
	return nil
}

// ResolveCLIToken authenticates a Bearer token and rechecks account state.
func (s *Service) ResolveCLIToken(
	ctx context.Context,
	token string,
) (Principal, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, cliTokenPrefix) || len(token) > 256 {
		return Principal{}, ErrUnauthenticated
	}
	hash := tokenHash(token)
	var principal Principal
	err := s.pool.QueryRow(ctx, `SELECT
		t.id,a.id,a.actor_type,a.display_name
		FROM cli_access_token t
		JOIN actor a ON a.id=t.actor_id
		LEFT JOIN local_account la ON la.actor_id=a.id
		WHERE t.token_hash=$1 AND t.revoked_at IS NULL AND t.expires_at>now()
		  AND a.status='active' AND (la.id IS NULL OR la.status='active')`,
		hash[:],
	).Scan(
		&principal.CredentialID, &principal.ActorID,
		&principal.ActorType, &principal.DisplayName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("auth: resolve CLI token: %w", err)
	}
	principal.Method = "cli_token"
	if _, err := s.pool.Exec(ctx, `UPDATE cli_access_token
		SET last_used_at=now()
		WHERE id=$1
		  AND (last_used_at IS NULL OR
		       last_used_at<now()-make_interval(secs => $2))`,
		principal.CredentialID,
		int(cliTokenUsageInterval/time.Second),
	); err != nil {
		return Principal{}, fmt.Errorf("auth: update CLI token usage: %w", err)
	}
	return principal, nil
}

func validateCLITokenName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > 120 {
		return "", ErrInvalidCLIInput
	}
	return value, nil
}

func redactedCLITokenPrefix(value string) string {
	const visible = 12
	if len(value) <= visible {
		return value
	}
	return value[:visible]
}

func cliTokenStatus(value CLIToken, now time.Time) string {
	if value.RevokedAt != nil {
		return "revoked"
	}
	if !value.ExpiresAt.After(now) {
		return "expired"
	}
	return "active"
}
