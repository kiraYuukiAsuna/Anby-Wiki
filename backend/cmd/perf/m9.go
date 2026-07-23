package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/collection"
	"github.com/anby/wiki/backend/internal/component"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/projection"
)

type m9Fixtures struct {
	ComponentID  uuid.UUID
	CollectionID uuid.UUID
	EntityIDs    []uuid.UUID
}

func seedM9(ctx context.Context, pool *pgxpool.Pool, pages []fixture) (m9Fixtures, error) {
	count := min(len(pages), 1000)
	if count == 0 {
		return m9Fixtures{}, fmt.Errorf("perf: M9 fixtures require at least one page")
	}

	ids := id.NewGenerator()
	txm := db.NewTxManager(pool)
	pageRepo := page.NewRepository(pool)
	knowledgeService := knowledge.NewService(knowledge.NewRepository(pool), pageRepo, txm, ids)
	componentService := component.NewService(
		component.NewRepository(pool), pageRepo, txm, ids, component.NewDefaultRegistry(),
	)
	collectionService := collection.NewService(collection.NewRepository(pool), pageRepo, txm, ids)
	resourceService := evidence.NewExternalResourceService(evidence.NewRepository(pool), ids)
	pageService := page.NewService(pageRepo, txm, ids)

	definition, err := componentService.Create(ctx, component.CreateParams{
		ComponentKey: "perf.card", Name: "Performance Card", ActorID: defaultSystemActor,
	})
	if err != nil {
		return m9Fixtures{}, err
	}
	version, err := componentService.CreateVersion(ctx, component.CreateVersionParams{
		ComponentID: definition.ID,
		PropsSchema: json.RawMessage(`{"type":"object","additionalProperties":{"type":"string"}}`),
		RendererRef: "builtin.key_value",
		ActorID:     defaultSystemActor,
	})
	if err != nil {
		return m9Fixtures{}, err
	}
	if _, err := componentService.Publish(ctx, definition.ID, version.Version, defaultSystemActor); err != nil {
		return m9Fixtures{}, err
	}

	entities := make([]uuid.UUID, 0, count)
	members := make([]collection.MemberInput, 0, count)
	projections := projection.NewRegistry()
	projections.Register(projection.NewComponentDependencyBuilder(pool))
	rebuilder := projection.NewRebuilder(pool, projections, nil)
	for index := 0; index < count; index++ {
		entity, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
			WikiID: defaultWikiID, TypeKey: "character",
			CanonicalKey: fmt.Sprintf("perf-entity-%06d", index),
			Labels: []knowledge.LabelInput{{
				Language: "zh-Hans", Label: fmt.Sprintf("性能实体 %06d", index), IsPrimary: true,
			}},
			ActorID: defaultSystemActor,
		})
		if err != nil {
			return m9Fixtures{}, err
		}
		entities = append(entities, entity.ID)
		componentAST := json.RawMessage(fmt.Sprintf(
			`{"type":"document","schema_version":1,"children":[{"id":%q,"type":"component","component_id":%q,"component_version":1,"entity_id":%q,"display_config":{"label":%q}}]}`,
			uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("perf-component-%d", index))),
			definition.ID, entity.ID, fmt.Sprintf("entity-%06d", index),
		))
		revision, err := pageService.Publish(ctx, page.PublishParams{
			PageID: pages[index].PageID, ActorID: defaultSystemActor,
			ExpectedRevisionID: &pages[index].RevisionID, AST: componentAST,
			Summary: "M9 performance fixture",
		})
		if err != nil {
			return m9Fixtures{}, err
		}
		pages[index].RevisionID = revision.ID
		if rebuilt, err := rebuilder.RebuildPage(ctx, pages[index].PageID); err != nil || !rebuilt {
			return m9Fixtures{}, fmt.Errorf("perf: rebuild component dependency: rebuilt=%t: %w", rebuilt, err)
		}
		entityID := entity.ID
		members = append(members, collection.MemberInput{
			MemberType: collection.MemberEntity, EntityID: &entityID,
			SourceRevisionID: pages[index].RevisionID, SortKey: fmt.Sprintf("%06d", index),
		})
		if _, err := resourceService.Upsert(ctx,
			fmt.Sprintf("https://perf-%06d.example.com/resource", index)); err != nil {
			return m9Fixtures{}, err
		}
	}

	set, err := collectionService.Create(ctx, collection.CreateParams{
		WikiID: defaultWikiID, CollectionType: collection.TypeManual,
		Title: "Performance Collection", ActorID: defaultSystemActor,
	})
	if err != nil {
		return m9Fixtures{}, err
	}
	if err := collectionService.ReplaceManualMembers(ctx, set.ID, defaultSystemActor, members); err != nil {
		return m9Fixtures{}, err
	}
	return m9Fixtures{ComponentID: definition.ID, CollectionID: set.ID, EntityIDs: entities}, nil
}
