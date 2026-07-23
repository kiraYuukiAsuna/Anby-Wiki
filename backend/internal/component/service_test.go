package component_test

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/anby/wiki/backend/internal/component"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

var cardSchema = json.RawMessage(`{
	"type":"object",
	"required":["title"],
	"additionalProperties":false,
	"properties":{
		"title":{"type":"string","minLength":1},
		"count":{"type":"integer","minimum":0}
	}
}`)

func newService(tdb *testkit.DB) *component.Service {
	pageRepo := page.NewRepository(tdb.Pool)
	return component.NewService(
		component.NewRepository(tdb.Pool),
		pageRepo,
		db.NewTxManager(tdb.Pool),
		id.NewGenerator(),
		component.NewDefaultRegistry(),
	)
}

func TestComponentVersionLifecycleValidationAndFreeze(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "component-author")
	service := newService(tdb)

	definition, err := service.Create(ctx, component.CreateParams{
		ComponentKey: "character.card", Name: "Character Card", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if definition.ComponentKey != "character.card" {
		t.Fatalf("component=%+v", definition)
	}

	version, err := service.CreateVersion(ctx, component.CreateVersionParams{
		ComponentID: definition.ID, PropsSchema: cardSchema,
		RendererRef: "builtin.key_value", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if version.Version != 1 || version.Status != component.StatusDraft {
		t.Fatalf("version=%+v", version)
	}
	if _, err := service.ValidateProps(
		ctx, definition.ID, 1, json.RawMessage(`{"title":"Anby"}`),
	); !errors.Is(err, component.ErrVersionFrozen) {
		t.Fatalf("draft validation err=%v", err)
	}

	published, err := service.Publish(ctx, definition.ID, 1, actorID)
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != component.StatusPublished || published.PublishedAt == nil {
		t.Fatalf("published=%+v", published)
	}
	if _, err := service.ValidateProps(
		ctx, definition.ID, 1, json.RawMessage(`{"title":"Anby","count":2}`),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ValidateProps(
		ctx, definition.ID, 1, json.RawMessage(`{"count":-1}`),
	); !errors.Is(err, component.ErrInvalidProps) {
		t.Fatalf("invalid props err=%v", err)
	}

	html, err := service.Render(
		ctx, definition.ID, 1,
		json.RawMessage(`{"title":"<script>alert(1)</script>","count":2}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>") || !strings.Contains(html, `&lt;script&gt;`) {
		t.Fatalf("unsafe renderer output: %s", html)
	}

	if _, err := service.UpdateDraft(
		ctx, definition.ID, 1, cardSchema, "builtin.key_value", actorID,
	); !errors.Is(err, component.ErrVersionFrozen) {
		t.Fatalf("update published err=%v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE component_version
		SET renderer_ref='untrusted.dynamic' WHERE component_id=$1 AND version=1`,
		definition.ID); err == nil {
		t.Fatal("database trigger allowed published version mutation")
	}
	if _, err := tdb.Pool.Exec(ctx, `DELETE FROM component_version
		WHERE component_id=$1 AND version=1`, definition.ID); err == nil {
		t.Fatal("database trigger allowed published version deletion")
	}

	deprecated, err := service.Deprecate(ctx, definition.ID, 1, actorID)
	if err != nil || deprecated.Status != component.StatusDeprecated {
		t.Fatalf("deprecated=%+v err=%v", deprecated, err)
	}
	if _, err := service.Render(
		ctx, definition.ID, 1, json.RawMessage(`{"title":"historical"}`),
	); err != nil {
		t.Fatalf("deprecated historical version must remain renderable: %v", err)
	}
}

func TestComponentVersionRejectsInvalidSchemaRendererAndActor(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	humanID := tdb.MakeActor(t, "human", "component-author")
	aiID := tdb.MakeActor(t, "ai", "component-ai")
	service := newService(tdb)

	if _, err := service.Create(ctx, component.CreateParams{
		ComponentKey: "bad key", Name: "Bad", ActorID: humanID,
	}); !errors.Is(err, component.ErrInvalidDefinition) {
		t.Fatalf("invalid key err=%v", err)
	}
	if _, err := service.Create(ctx, component.CreateParams{
		ComponentKey: "ai.card", Name: "AI Card", ActorID: aiID,
	}); !errors.Is(err, page.ErrActorNotAllowed) {
		t.Fatalf("ai create err=%v", err)
	}
	definition, err := service.Create(ctx, component.CreateParams{
		ComponentKey: "valid.card", Name: "Valid", ActorID: humanID,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, params := range []component.CreateVersionParams{
		{
			ComponentID: definition.ID,
			PropsSchema: json.RawMessage(`{"type":"string"}`),
			RendererRef: "builtin.key_value", ActorID: humanID,
		},
		{
			ComponentID: definition.ID, PropsSchema: cardSchema,
			RendererRef: "plugin.from_database", ActorID: humanID,
		},
	} {
		if _, err := service.CreateVersion(ctx, params); err == nil {
			t.Fatalf("invalid version accepted: %+v", params)
		}
	}
}

func TestConcurrentComponentVersionsReceiveUniqueSequence(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "component-concurrency")
	service := newService(tdb)
	definition, err := service.Create(ctx, component.CreateParams{
		ComponentKey: "concurrent.card", Name: "Concurrent", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}

	const count = 8
	versions := make([]int, 0, count)
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := service.CreateVersion(ctx, component.CreateVersionParams{
				ComponentID: definition.ID, PropsSchema: cardSchema,
				RendererRef: "builtin.key_value", ActorID: actorID,
			})
			if err != nil {
				errs <- err
				return
			}
			mu.Lock()
			versions = append(versions, value.Version)
			mu.Unlock()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	sort.Ints(versions)
	for index, version := range versions {
		if version != index+1 {
			t.Fatalf("versions=%v", versions)
		}
	}
}
