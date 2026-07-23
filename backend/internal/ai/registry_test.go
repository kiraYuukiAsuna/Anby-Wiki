package ai_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestPromptRegistry_VersionAndAtomicActivation(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	repo := ai.NewRepository(tdb.Pool)
	registry := ai.NewRegistry(repo, db.NewTxManager(tdb.Pool), id.NewGenerator())
	schema := json.RawMessage(`{"type":"object"}`)
	first, err := registry.Register(context.Background(), "extract", 1, "system", "{{.text}}", schema, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.Register(context.Background(), "extract", 2, "system v2", "{{.text}}", schema, true)
	if err != nil {
		t.Fatal(err)
	}
	active, err := repo.ActivePrompt(context.Background(), "extract")
	if err != nil {
		t.Fatal(err)
	}
	old, err := repo.PromptVersion(context.Background(), "extract", 1)
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != second.ID || active.Version != 2 || old.Active || first.ContentHash == second.ContentHash {
		t.Fatalf("first=%+v second=%+v active=%+v old=%+v", first, second, active, old)
	}
}
