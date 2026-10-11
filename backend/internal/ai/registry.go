package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

type Registry struct {
	repo *Repository
	txm  *db.TxManager
	ids  *id.Generator
}

func NewRegistry(repo *Repository, txm *db.TxManager, ids *id.Generator) *Registry {
	return &Registry{repo: repo, txm: txm, ids: ids}
}

func (r *Registry) Register(ctx context.Context, key string, version int, system, user string, schema json.RawMessage, activate bool) (*Prompt, error) {
	key = strings.TrimSpace(key)
	if key == "" || version <= 0 || strings.TrimSpace(user) == "" {
		return nil, ErrInvalidPrompt
	}
	var schemaObject map[string]any
	if err := json.Unmarshal(schema, &schemaObject); err != nil || schemaObject == nil {
		return nil, fmt.Errorf("%w: output schema", ErrInvalidPrompt)
	}
	if _, err := template.New("system").Option("missingkey=error").Parse(system); err != nil {
		return nil, fmt.Errorf("%w: system template: %v", ErrInvalidPrompt, err)
	}
	if _, err := template.New("user").Option("missingkey=error").Parse(user); err != nil {
		return nil, fmt.Errorf("%w: user template: %v", ErrInvalidPrompt, err)
	}
	hashInput, _ := json.Marshal([]any{key, version, system, user, schemaObject})
	sum := sha256.Sum256(hashInput)
	id, err := r.ids.New()
	if err != nil {
		return nil, err
	}
	p := &Prompt{ID: id, Key: key, Version: version, System: system, User: user,
		OutputSchema: schema, ContentHash: hex.EncodeToString(sum[:])}
	err = r.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := r.repo.InsertPrompt(ctx, tx, p); err != nil {
			return err
		}
		if activate {
			if err := r.repo.ActivatePrompt(ctx, tx, p.ID, p.Key); err != nil {
				return err
			}
			p.Active = true
		}
		return nil
	})
	return p, err
}

// EnsureActive creates and activates a requested bundled prompt version when
// the current active version is older. A newer operator-selected version is
// preserved. Callers must increment version whenever bundled content changes.
func (r *Registry) EnsureActive(ctx context.Context, key string, version int, system, user string, schema json.RawMessage) (*Prompt, error) {
	if active, err := r.repo.ActivePrompt(ctx, key); err == nil && active.Version >= version {
		return active, nil
	} else if err != nil && !errors.Is(err, ErrPromptNotFound) {
		return nil, err
	}

	requested, err := r.repo.PromptVersion(ctx, key, version)
	if err != nil && !errors.Is(err, ErrPromptNotFound) {
		return nil, err
	}
	if errors.Is(err, ErrPromptNotFound) {
		requested, err = r.Register(ctx, key, version, system, user, schema, false)
	}
	if err != nil {
		// A concurrent Worker may have inserted the same bootstrap version.
		requested, err = r.repo.PromptVersion(ctx, key, version)
		if err != nil {
			return nil, err
		}
	}

	err = r.txm.InTx(ctx, func(tx pgx.Tx) error {
		active, activeErr := r.repo.activePromptForUpdate(ctx, tx, key)
		switch {
		case activeErr == nil && active.Version >= version:
			return nil
		case activeErr == nil:
			return r.repo.ActivatePrompt(ctx, tx, requested.ID, key)
		case !errors.Is(activeErr, ErrPromptNotFound):
			return activeErr
		default:
			_, activateErr := r.repo.ActivatePromptIfNone(ctx, tx, requested.ID, key)
			return activateErr
		}
	})
	if err != nil {
		return nil, err
	}
	return r.repo.ActivePrompt(ctx, key)
}

func renderPrompt(source string, variables map[string]any) (string, error) {
	t, err := template.New("prompt").Option("missingkey=error").Parse(source)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, variables); err != nil {
		return "", err
	}
	return out.String(), nil
}
