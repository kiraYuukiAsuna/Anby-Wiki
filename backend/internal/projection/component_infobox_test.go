package projection_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/component"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

func TestComponentInfoboxProjectionAndPreciseClaimRerender(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ctx := context.Background()
	actorID := d.MakeActor(t, "human", "m9-t03")

	knowledgeService := knowledge.NewService(
		knowledge.NewRepository(d.Pool),
		page.NewRepository(d.Pool),
		db.NewTxManager(d.Pool),
		id.NewGenerator(),
	)
	entity, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID:       testkit.DefaultWikiID,
		TypeKey:      "character",
		CanonicalKey: "anby",
		Labels: []knowledge.LabelInput{{
			Language: "zh-Hans", Label: "安比<script>", IsPrimary: true,
		}},
		ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	d.MakeProperty(t, "profile_note", "string", true, nil, nil, "")
	firstClaim, err := knowledgeService.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: entity.ID,
		PropertyKey:     "profile_note",
		Value:           knowledge.StringValue(`<img src=x onerror=alert(1)>`),
		OriginType:      knowledge.OriginHuman,
		ActorID:         actorID,
	})
	if err != nil {
		t.Fatal(err)
	}

	componentService := component.NewService(
		component.NewRepository(d.Pool),
		page.NewRepository(d.Pool),
		db.NewTxManager(d.Pool),
		id.NewGenerator(),
		component.NewKnowledgeRegistry(d.Pool),
	)
	definition, err := componentService.Create(ctx, component.CreateParams{
		ComponentKey: "entity.infobox", Name: "Entity Infobox", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	version, err := componentService.CreateVersion(ctx, component.CreateVersionParams{
		ComponentID: definition.ID,
		PropsSchema: json.RawMessage(`{
			"type":"object",
			"required":["title","language","property_keys"],
			"additionalProperties":false,
			"properties":{
				"title":{"type":"string"},
				"language":{"type":"string"},
				"property_keys":{"type":"array","items":{"type":"string"}}
			}
		}`),
		RendererRef: component.RendererEntityClaimInfobox,
		ActorID:     actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := componentService.Publish(ctx, definition.ID, version.Version, actorID); err != nil {
		t.Fatal(err)
	}

	pageService := newPageService(d)
	pageID := d.MakePage(t, testkit.MainNamespaceID, "anby", "安比", actorID)
	config := json.RawMessage(`{
		"title":"<script>角色</script>",
		"language":"zh-Hans",
		"property_keys":["profile_note"]
	}`)
	componentDoc := &ast.Document{
		Type: "document", SchemaVersion: 1,
		Children: []*ast.Block{{
			ID: d.NewID(t).String(), Type: ast.BlockComponent,
			ComponentID: definition.ID.String(), ComponentVersion: version.Version,
			EntityID: entity.ID.String(), DisplayConfig: config,
		}},
	}
	componentJSON, err := json.Marshal(componentDoc)
	if err != nil {
		t.Fatal(err)
	}
	revisionID := publishAST(t, pageService, pageID, actorID, string(componentJSON))

	registry := projection.NewRegistry()
	registry.Register(projection.NewComponentDependencyBuilder(d.Pool))
	registry.Register(projection.NewClaimUsageBuilder(d.Pool))
	registry.Register(projection.NewRenderedPageBuilder(d.Pool))
	dispatchPublishEvent(t, d, registry, pageID, revisionID)

	rendered, ok := readRenderedRow(t, d, pageID)
	if !ok {
		t.Fatal("信息框页面应生成 rendered_page")
	}
	for _, unsafe := range []string{"<script>", "<img"} {
		if strings.Contains(rendered.html, unsafe) {
			t.Fatalf("信息框输出未转义 %q: %s", unsafe, rendered.html)
		}
	}
	for _, escaped := range []string{"&lt;script&gt;", "&lt;img"} {
		if !strings.Contains(rendered.html, escaped) {
			t.Fatalf("信息框输出缺少转义文本 %q: %s", escaped, rendered.html)
		}
	}

	var dependencyCount, usageCount int
	if err := d.Pool.QueryRow(ctx, `
		SELECT count(*) FROM component_dependency
		WHERE page_id=$1 AND revision_id=$2 AND component_id=$3
		  AND component_version=$4 AND entity_id=$5`,
		pageID, revisionID, definition.ID, version.Version, entity.ID,
	).Scan(&dependencyCount); err != nil {
		t.Fatal(err)
	}
	if err := d.Pool.QueryRow(ctx, `
		SELECT count(*) FROM claim_usage
		WHERE page_id=$1 AND revision_id=$2 AND claim_id=$3`,
		pageID, revisionID, firstClaim.ID,
	).Scan(&usageCount); err != nil {
		t.Fatal(err)
	}
	if dependencyCount != 1 || usageCount != 1 {
		t.Fatalf("component_dependency=%d claim_usage=%d, want 1/1", dependencyCount, usageCount)
	}

	unrelatedID := d.MakePage(t, testkit.MainNamespaceID, "unrelated", "Unrelated", actorID)
	unrelatedRevision := publishAST(t, pageService, unrelatedID, actorID,
		document(paragraphBlock(d.NewID(t).String(), textNode("unchanged"))))
	dispatchPublishEvent(t, d, newRenderRegistry(d), unrelatedID, unrelatedRevision)
	unrelatedBefore, _ := readRenderedRow(t, d, unrelatedID)

	secondClaim, err := knowledgeService.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: entity.ID,
		PropertyKey:     "profile_note",
		Value:           knowledge.StringValue("第二条"),
		OriginType:      knowledge.OriginHuman,
		ActorID:         actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	event := latestClaimChangedEvent(t, d, secondClaim.ID)
	handler := projection.NewClaimChangedHandler(d.Pool, nil)
	if err := handler.Handle(ctx, event); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(ctx, event); err != nil {
		t.Fatalf("重复 Claim 事件应幂等: %v", err)
	}

	updated, _ := readRenderedRow(t, d, pageID)
	if !strings.Contains(updated.html, "第二条") {
		t.Fatalf("受影响页未渲染新 Claim: %s", updated.html)
	}
	unrelatedAfter, _ := readRenderedRow(t, d, unrelatedID)
	if unrelatedAfter != unrelatedBefore {
		t.Fatalf("无依赖页面不应重渲染\n before=%+v\n after=%+v", unrelatedBefore, unrelatedAfter)
	}
}

func latestClaimChangedEvent(
	t *testing.T, d *testkit.DB, claimID uuid.UUID,
) projection.Event {
	t.Helper()
	var event projection.Event
	if err := d.Pool.QueryRow(context.Background(), `
		SELECT id, aggregate_type, aggregate_id, event_type, payload_json, created_at
		FROM outbox_event
		WHERE aggregate_id=$1 AND event_type=$2
		ORDER BY created_at DESC, id DESC LIMIT 1`,
		claimID, knowledge.OutboxEventClaimChanged,
	).Scan(
		&event.ID, &event.AggregateType, &event.AggregateID,
		&event.EventType, &event.Payload, &event.CreatedAt,
	); err != nil {
		t.Fatal(err)
	}
	event.IdempotencyKey = event.ID.String()
	return event
}
