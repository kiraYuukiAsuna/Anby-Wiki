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
	ErrInvalidCLIInput              = errors.New("auth: invalid CLI authorization input")
	ErrInvalidCLIAuthorizationCode  = errors.New("auth: invalid CLI authorization code")
	ErrCLIAuthorizationCodeNotFound = errors.New("auth: CLI authorization code not found")
	ErrCLITokenNotFound             = errors.New("auth: CLI token not found")
	ErrActiveCLITokenDeletion       = errors.New("auth: active CLI token cannot be deleted")
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
	CLIActorIdentity
	Code           string
	Name           string
	ExpiresAt      time.Time
	TokenExpiresAt time.Time
}

// CLIAuthorizationCodeSummary is an unredeemed code audit entry. The
// plaintext code is intentionally unavailable after creation.
type CLIAuthorizationCodeSummary struct {
	CLIActorIdentity
	ID             uuid.UUID
	Name           string
	ExpiresAt      time.Time
	TokenExpiresAt time.Time
	CreatedAt      time.Time
}

// CLIToken is the redacted token record exposed to the account that owns it.
type CLIToken struct {
	CLIActorIdentity
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
	CLIActorIdentity
	Token     string
	TokenInfo CLIToken
}

type CreateCLIAuthCodeParams struct {
	IssuedBy     uuid.UUID
	ActorID      uuid.UUID
	Name         string
	TokenTTLDays int
}

func (s *Service) CreateCLIAuthorizationCode(
	ctx context.Context,
	in CreateCLIAuthCodeParams,
) (CLIAuthorizationCode, error) {
	name, err := validateCLITokenName(in.Name)
	actorID, tokenTTLDays := in.ActorID, in.TokenTTLDays
	if actorID == uuid.Nil {
		actorID = in.IssuedBy
	}
	if err != nil || in.IssuedBy == uuid.Nil ||
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
		identity, err := lockCLIActor(ctx, tx, actorID, in.IssuedBy)
		if err != nil {
			return err
		}
		result.CLIActorIdentity = identity
		if _, err := tx.Exec(ctx, `DELETE FROM cli_authorization_code
			WHERE actor_id=$1 AND (used_at IS NOT NULL OR expires_at<=now())`,
			actorID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO cli_authorization_code
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
		var actorID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT actor_id FROM cli_authorization_code
			WHERE code_hash=$1 AND used_at IS NULL AND expires_at>now()`, codeHash[:]).Scan(&actorID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidCLIAuthorizationCode
			}
			return err
		}
		identity, err := lockCLIActor(ctx, tx, actorID, uuid.Nil)
		if errors.Is(err, ErrUnauthenticated) {
			return ErrInvalidCLIAuthorizationCode
		}
		if err != nil {
			return err
		}
		result.CLIActorIdentity = identity
		var codeID uuid.UUID
		var name string
		var expiresAt time.Time
		err = tx.QueryRow(ctx, `SELECT
			c.id,c.token_name,c.token_expires_at
			FROM cli_authorization_code c
			WHERE c.code_hash=$1 AND c.used_at IS NULL AND c.expires_at>now()
			FOR UPDATE OF c`,
			codeHash[:],
		).Scan(&codeID, &name, &expiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidCLIAuthorizationCode
		}
		if err != nil {
			return err
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
			CLIActorIdentity: identity,
			ID:               tokenID, Name: name, TokenPrefix: prefix,
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

func (s *Service) ListCLIAuthorizationCodes(
	ctx context.Context,
	actorID uuid.UUID,
) ([]CLIAuthorizationCodeSummary, error) {
	if actorID == uuid.Nil {
		return nil, ErrUnauthenticated
	}
	rows, err := s.pool.Query(ctx, `SELECT
		c.id,c.token_name,c.expires_at,c.token_expires_at,c.created_at,
		a.id,COALESCE(a.wiki_id,$2),a.actor_type,a.display_name
		FROM cli_authorization_code c JOIN actor a ON a.id=c.actor_id
		LEFT JOIN local_account la ON la.id=a.user_id
		WHERE (c.actor_id=$1 OR (a.actor_type IN ('ai','import') AND la.actor_id=$1))
		  AND c.used_at IS NULL AND c.expires_at>now()
		ORDER BY c.created_at DESC,c.id DESC`, actorID, uuid.Nil)
	if err != nil {
		return nil, fmt.Errorf("auth: list CLI authorization codes: %w", err)
	}
	defer rows.Close()
	result := []CLIAuthorizationCodeSummary{}
	for rows.Next() {
		var item CLIAuthorizationCodeSummary
		if err := rows.Scan(
			&item.ID, &item.Name, &item.ExpiresAt,
			&item.TokenExpiresAt, &item.CreatedAt,
			&item.ActorID, &item.WikiID, &item.ActorType, &item.DisplayName,
		); err != nil {
			return nil, fmt.Errorf("auth: scan CLI authorization code: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: iterate CLI authorization codes: %w", err)
	}
	return result, nil
}

func (s *Service) DeleteCLIAuthorizationCode(
	ctx context.Context,
	actorID, codeID uuid.UUID,
) error {
	if actorID == uuid.Nil || codeID == uuid.Nil {
		return ErrCLIAuthorizationCodeNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM cli_authorization_code c USING actor a
		LEFT JOIN local_account la ON la.id=a.user_id
		WHERE c.actor_id=a.id AND c.id=$1
		  AND (c.actor_id=$2 OR (a.actor_type IN ('ai','import') AND la.actor_id=$2))
		  AND c.used_at IS NULL AND c.expires_at>now()`,
		codeID, actorID)
	if err != nil {
		return fmt.Errorf("auth: delete CLI authorization code: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrCLIAuthorizationCodeNotFound
	}
	return nil
}

func (s *Service) ListCLITokens(
	ctx context.Context,
	actorID uuid.UUID,
) ([]CLIToken, error) {
	if actorID == uuid.Nil {
		return nil, ErrUnauthenticated
	}
	rows, err := s.pool.Query(ctx, `SELECT
		t.id,t.name,t.token_prefix,t.expires_at,t.last_used_at,t.revoked_at,t.created_at,
		a.id,COALESCE(a.wiki_id,$2),a.actor_type,a.display_name
		FROM cli_access_token t JOIN actor a ON a.id=t.actor_id
		LEFT JOIN local_account la ON la.id=a.user_id
		WHERE t.actor_id=$1 OR (a.actor_type IN ('ai','import') AND la.actor_id=$1)
		ORDER BY t.created_at DESC,t.id DESC`, actorID, uuid.Nil)
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
			&item.ActorID, &item.WikiID, &item.ActorType, &item.DisplayName,
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
	tag, err := s.pool.Exec(ctx, `UPDATE cli_access_token t
		SET revoked_at=COALESCE(t.revoked_at,now()) FROM actor a
		LEFT JOIN local_account la ON la.id=a.user_id
		WHERE t.actor_id=a.id AND t.id=$1
		  AND (t.actor_id=$2 OR (a.actor_type IN ('ai','import') AND la.actor_id=$2))`, tokenID, actorID)
	if err != nil {
		return fmt.Errorf("auth: revoke CLI token: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrCLITokenNotFound
	}
	return nil
}

func (s *Service) DeleteCLITokenRecord(
	ctx context.Context,
	actorID, tokenID uuid.UUID,
) error {
	if actorID == uuid.Nil || tokenID == uuid.Nil {
		return ErrCLITokenNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM cli_access_token t USING actor a
		LEFT JOIN local_account la ON la.id=a.user_id
		WHERE t.actor_id=a.id AND t.id=$1
		  AND (t.actor_id=$2 OR (a.actor_type IN ('ai','import') AND la.actor_id=$2))
		  AND (t.revoked_at IS NOT NULL OR t.expires_at<=now())`,
		tokenID, actorID)
	if err != nil {
		return fmt.Errorf("auth: delete CLI token record: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM cli_access_token t JOIN actor a ON a.id=t.actor_id
		LEFT JOIN local_account la ON la.id=a.user_id
		WHERE t.id=$1 AND (t.actor_id=$2 OR (a.actor_type IN ('ai','import') AND la.actor_id=$2))
	)`, tokenID, actorID).Scan(&exists); err != nil {
		return fmt.Errorf("auth: inspect CLI token record: %w", err)
	}
	if !exists {
		return ErrCLITokenNotFound
	}
	return ErrActiveCLITokenDeletion
}

func (s *Service) DeleteInactiveCLITokenRecords(
	ctx context.Context,
	actorID uuid.UUID,
) (int64, error) {
	if actorID == uuid.Nil {
		return 0, ErrUnauthenticated
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM cli_access_token t USING actor a
		LEFT JOIN local_account la ON la.id=a.user_id
		WHERE t.actor_id=a.id AND (t.actor_id=$1 OR (a.actor_type IN ('ai','import') AND la.actor_id=$1))
		  AND (t.revoked_at IS NOT NULL OR t.expires_at<=now())`,
		actorID)
	if err != nil {
		return 0, fmt.Errorf("auth: delete inactive CLI token records: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ResolveCLIToken authenticates a Bearer token and rechecks account state.
func (s *Service) ResolveCLIToken(
	ctx context.Context,
	token string,
	requestedWikiID uuid.UUID,
) (Principal, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, cliTokenPrefix) || len(token) > 256 {
		return Principal{}, ErrUnauthenticated
	}
	hash := tokenHash(token)
	var credentialID, actorID uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT
		t.id,t.actor_id
		FROM cli_access_token t
		JOIN actor a ON a.id=t.actor_id
		LEFT JOIN local_account la ON la.actor_id=a.id
		WHERE t.token_hash=$1 AND t.revoked_at IS NULL AND t.expires_at>now()
		  AND a.status='active' AND (la.id IS NULL OR la.status='active')`,
		hash[:],
	).Scan(&credentialID, &actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("auth: resolve CLI token: %w", err)
	}
	principal, err := s.resolveActorPrincipal(
		ctx, actorID, requestedWikiID, credentialID, "cli_token",
	)
	if err != nil {
		return Principal{}, err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE cli_access_token
		SET last_used_at=now()
		WHERE id=$1
		  AND (last_used_at IS NULL OR
		       last_used_at<now()-make_interval(secs => $2))`,
		credentialID,
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
