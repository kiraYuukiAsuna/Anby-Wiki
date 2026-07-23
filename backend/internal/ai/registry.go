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

// EnsureActive returns an operator-selected active version when one exists;
// otherwise it activates or creates the requested bootstrap version. This is
// safe for concurrent Worker startup and never replaces a newer active prompt.
func (r *Registry) EnsureActive(ctx context.Context, key string, version int, system, user string, schema json.RawMessage) (*Prompt, error) {
	if active, err := r.repo.ActivePrompt(ctx, key); err == nil {
		return active, nil
	} else if !errors.Is(err, ErrPromptNotFound) {
		return nil, err
	}
	if existing, err := r.repo.PromptVersion(ctx, key, version); err == nil {
		activated := false
		err = r.txm.InTx(ctx, func(tx pgx.Tx) error {
			var err error
			activated, err = r.repo.ActivatePromptIfNone(ctx, tx, existing.ID, key)
			return err
		})
		if err == nil && activated {
			existing.Active = true
			return existing, nil
		}
		if err == nil {
			return r.repo.ActivePrompt(ctx, key)
		}
	} else if !errors.Is(err, ErrPromptNotFound) {
		return nil, err
	}
	created, err := r.Register(ctx, key, version, system, user, schema, false)
	if err != nil {
		// A concurrent Worker may have inserted the same bootstrap version.
		created, err = r.repo.PromptVersion(ctx, key, version)
		if err != nil {
			return nil, err
		}
	}
	activated := false
	err = r.txm.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		activated, err = r.repo.ActivatePromptIfNone(ctx, tx, created.ID, key)
		return err
	})
	if err != nil {
		return nil, err
	}
	if activated {
		created.Active = true
		return created, nil
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
