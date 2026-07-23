package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/importer"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/projection"
	wikisearch "github.com/anby/wiki/backend/internal/search"
)

func benchmark(
	ctx context.Context,
	pool *pgxpool.Pool,
	cfg config,
	fixtures []fixture,
	m9 m9Fixtures,
) (report, error) {
	pages := page.NewService(page.NewRepository(pool), db.NewTxManager(pool), id.NewGenerator())
	search, searchBackend, searchIndexDuration, err := performanceSearch(ctx, pool)
	if err != nil {
		return report{}, err
	}
	relations := projection.NewQueries(pool)
	imports := importer.NewService(importer.NewRepository(pool), db.NewTxManager(pool), id.NewGenerator())
	importJob, err := seedWorkflowFixtures(ctx, pool, imports, fixtures, min(1000, len(fixtures)))
	if err != nil {
		return report{}, err
	}
	publishPage, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: defaultWikiID, NamespaceID: mainNamespaceID,
		Title: "性能发布基准", ActorID: defaultSystemActor,
	})
	if err != nil {
		return report{}, err
	}

	var currentRevision uuid.UUID
	var publishMu sync.Mutex
	operations := []struct {
		name string
		fn   func(int) error
	}{
		{"read_current_revision", func(index int) error {
			_, _, err := pages.CurrentContent(ctx, fixtures[index%len(fixtures)].PageID)
			return err
		}},
		{"search_fts_common_term", func(int) error {
			_, _, err := search.Search(ctx, wikisearch.Query{
				Text: "共同基准词", WikiID: defaultWikiID, Namespace: "main", Limit: 20,
			})
			return err
		}},
		{"backlinks", func(int) error {
			_, err := relations.Backlinks(ctx, fixtures[0].PageID, "", 20)
			return err
		}},
		{"publish_revision", func(index int) error {
			publishMu.Lock()
			defer publishMu.Unlock()
			var expected *uuid.UUID
			if currentRevision != uuid.Nil {
				expected = &currentRevision
			}
			rev, err := pages.Publish(ctx, page.PublishParams{
				PageID: publishPage.ID, ActorID: defaultSystemActor, ExpectedRevisionID: expected,
				AST: astFor(cfg.Pages+1, index), Summary: "performance benchmark",
			})
			if err == nil {
				currentRevision = rev.ID
			}
			return err
		}},
		{"review_queue", func(int) error {
			rows, err := pool.Query(ctx, `SELECT id, proposal_id, created_at FROM review_task
				WHERE status='pending' ORDER BY created_at, id LIMIT 20`)
			if err == nil {
				rows.Close()
			}
			return err
		}},
		{"import_detail", func(int) error {
			_, err := imports.Detail(ctx, importJob.ID)
			return err
		}},
		{"component_version_lookup", func(int) error {
			var status string
			return pool.QueryRow(ctx, `SELECT status FROM component_version
				WHERE component_id=$1 AND version=1`, m9.ComponentID).Scan(&status)
		}},
		{"collection_members_page", func(int) error {
			rows, err := pool.Query(ctx, `SELECT entity_id,sort_key FROM collection_membership
				WHERE collection_id=$1 ORDER BY sort_key,member_type,page_id,entity_id LIMIT 20`, m9.CollectionID)
			if err == nil {
				rows.Close()
			}
			return err
		}},
		{"external_link_due_queue", func(int) error {
			rows, err := pool.Query(ctx, `SELECT id FROM external_resource
				WHERE next_check_at <= now() ORDER BY next_check_at,id LIMIT 20`)
			if err == nil {
				rows.Close()
			}
			return err
		}},
		{"entity_resolve", func(index int) error {
			var status string
			var mergedInto *uuid.UUID
			return pool.QueryRow(ctx, `SELECT status,merged_into_entity_id FROM entity WHERE id=$1`,
				m9.EntityIDs[index%len(m9.EntityIDs)]).Scan(&status, &mergedInto)
		}},
	}
	result := report{SearchBackend: searchBackend, SearchIndexSeconds: searchIndexDuration.Seconds()}
	for _, operation := range operations {
		result.Metrics = append(result.Metrics, measure(cfg.Iterations, operation.name, operation.fn))
	}
	result.Explains, err = collectExplains(ctx, pool, fixtures, m9)
	return result, err
}

func performanceSearch(ctx context.Context, pool *pgxpool.Pool) (wikisearch.SearchAdapter, string, time.Duration, error) {
	backend := os.Getenv("PERF_SEARCH_BACKEND")
	if backend == "" {
		backend = wikisearch.BackendPostgres
	}
	if backend == wikisearch.BackendPostgres {
		return wikisearch.NewPostgresAdapter(pool), backend, 0, nil
	}
	timeout := 15 * time.Second
	if raw := os.Getenv("MEILI_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return nil, backend, 0, fmt.Errorf("parse MEILI_TIMEOUT: %w", err)
		}
		timeout = parsed
	}
	adapter, err := wikisearch.NewBackend(pool, wikisearch.BackendConfig{
		Backend: backend, MeiliURL: os.Getenv("MEILI_URL"), MeiliAPIKey: os.Getenv("MEILI_API_KEY"),
		MeiliIndex: os.Getenv("MEILI_INDEX"), MeiliTimeout: timeout,
	})
	if err != nil {
		return nil, backend, 0, err
	}
	meili, ok := adapter.(*wikisearch.MeilisearchAdapter)
	if !ok {
		return nil, backend, 0, fmt.Errorf("unsupported performance search backend %q", backend)
	}
	if err := meili.EnsureIndex(ctx); err != nil {
		return nil, backend, 0, err
	}
	documents, err := wikisearch.NewPostgresAdapter(pool).AllStagedDocuments(ctx)
	if err != nil {
		return nil, backend, 0, err
	}
	started := time.Now()
	if err := meili.Rebuild(ctx, documents); err != nil {
		return nil, backend, 0, err
	}
	return meili, backend, time.Since(started), nil
}

func seedWorkflowFixtures(ctx context.Context, pool *pgxpool.Pool, imports *importer.Service, fixtures []fixture, count int) (*importer.Job, error) {
	repo := governance.NewRepository(pool)
	txm := db.NewTxManager(pool)
	ids := id.NewGenerator()
	proposals := governance.NewService(repo, txm, ids)
	reviews := governance.NewReviewService(repo, txm, ids, governance.NewRiskEvaluator(nil))
	var firstImport *importer.Job
	for index := 0; index < count; index++ {
		job, err := imports.Create(ctx, defaultSystemActor, "performance",
			fmt.Sprintf("perf-import-%06d", index), json.RawMessage(`{"source":"synthetic"}`))
		if err != nil {
			return nil, err
		}
		if firstImport == nil {
			firstImport = job
		}
		target := fixtures[index%len(fixtures)]
		proposal, err := proposals.CreateProposal(ctx, governance.CreateProposalParams{
			TargetType: governance.TargetPage, TargetID: &target.PageID,
			BaseRevisionID: &target.RevisionID, CreatedBy: defaultSystemActor,
			IdempotencyKey: fmt.Sprintf("perf-review-%06d", index),
		})
		if err != nil {
			return nil, err
		}
		operation := governance.OperationV1{
			SchemaVersion: 1, OperationType: governance.OpRenamePage,
			Base:     governance.OperationBase{RevisionID: &target.RevisionID},
			Target:   governance.OperationTarget{PageID: &target.PageID},
			Evidence: []governance.OperationEvidence{},
			Risk:     governance.OperationRisk{Level: governance.RiskLow, Reasons: []string{}},
			Payload:  json.RawMessage(fmt.Sprintf(`{"new_title":%q}`, target.Title+" 候选")),
		}
		raw, _ := json.Marshal(operation)
		if _, err := proposals.AddOperationV1(ctx, proposal.ID, raw); err != nil {
			return nil, err
		}
		if _, err := reviews.Submit(ctx, proposal.ID); err != nil {
			return nil, err
		}
	}
	return firstImport, nil
}

func collectDatabaseStats(ctx context.Context, pool *pgxpool.Pool, result *report) error {
	if err := pool.QueryRow(ctx, `SELECT version(), pg_database_size(current_database()),
		(SELECT count(*) FROM revision), (SELECT count(*) FROM search_document),
		(SELECT count(*) FROM page_link_projection)`).
		Scan(&result.Postgres, &result.DatabaseSize, &result.Revisions, &result.SearchDocs, &result.Relations); err != nil {
		return err
	}
	return pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM component),
		(SELECT count(*) FROM component_dependency),
		(SELECT count(*) FROM collection),
		(SELECT count(*) FROM collection_membership),
		(SELECT count(*) FROM external_resource),
		(SELECT count(*) FROM entity),
		(SELECT count(*) FROM external_resource WHERE next_check_at <= now()),
		(SELECT count(*) FROM outbox_event
		 WHERE event_type='claim.changed' AND status IN ('pending','claimed'))`).
		Scan(
			&result.M9.Components,
			&result.M9.ComponentDependencies,
			&result.M9.Collections,
			&result.M9.CollectionMemberships,
			&result.M9.ExternalResources,
			&result.M9.Entities,
			&result.M9.DueExternalLinks,
			&result.M9.PendingClaimChanges,
		)
}

func collectExplains(
	ctx context.Context,
	pool *pgxpool.Pool,
	fixtures []fixture,
	m9 m9Fixtures,
) ([]explain, error) {
	queries := []struct {
		name string
		sql  string
		args []any
	}{
		{"read_current_revision", `SELECT r.id,s.ast_json FROM page p
			JOIN revision r ON r.id=p.current_revision_id
			JOIN content_snapshot s ON s.id=r.content_snapshot_id WHERE p.id=$1`, []any{fixtures[0].PageID}},
		{"search_fts", `SELECT page_id FROM search_document
			WHERE search_vector @@ websearch_to_tsquery('simple',$1)
			ORDER BY ts_rank_cd(search_vector,websearch_to_tsquery('simple',$1)) DESC LIMIT 20`,
			[]any{"共同基准词"}},
		{"search_adapter_count", `WITH input AS (
				SELECT websearch_to_tsquery('simple',$1) AS tsq, lower($1) AS needle
			) SELECT count(*) FROM search_document d CROSS JOIN input i
			WHERE d.wiki_id=$2 AND d.namespace_key=$3
			  AND (
				to_tsvector('simple',d.display_title||' '||d.normalized_title) @@ i.tsq
				OR d.normalized_title ILIKE $4
				OR d.display_title ILIKE $4
				OR to_tsvector('simple',array_to_string(d.aliases,' ')) @@ i.tsq
				OR EXISTS (SELECT 1 FROM unnest(d.aliases) a WHERE a ILIKE $4)
				OR to_tsvector('simple',d.body_text) @@ i.tsq
				OR d.body_text ILIKE $4
				OR to_tsvector('simple',array_to_string(d.entity_terms,' ')) @@ i.tsq
			)`, []any{"共同基准词", defaultWikiID, "main", "%共同基准词%"}},
		{"backlinks", `SELECT source_page_id FROM page_link_projection
			WHERE target_page_id=$1 AND resolution_status='resolved'
			ORDER BY source_page_id,source_block_id,source_node_id LIMIT 21`, []any{fixtures[0].PageID}},
		{"review_queue", `SELECT id FROM review_task WHERE status='pending'
			ORDER BY created_at,id LIMIT 20`, nil},
		{"import_detail", `SELECT j.id,r.id,s.id FROM import_job j
			LEFT JOIN import_run r ON r.import_job_id=j.id
			LEFT JOIN import_stage_run s ON s.import_run_id=r.id WHERE j.id=$1`,
			[]any{uuid.Nil}},
		{"component_version_lookup", `SELECT status FROM component_version
			WHERE component_id=$1 AND version=1`, []any{m9.ComponentID}},
		{"collection_members_page", `SELECT entity_id,sort_key FROM collection_membership
			WHERE collection_id=$1 ORDER BY sort_key,member_type,page_id,entity_id LIMIT 20`,
			[]any{m9.CollectionID}},
		{"external_link_due_queue", `SELECT id FROM external_resource
			WHERE next_check_at <= now() ORDER BY next_check_at,id LIMIT 20`, nil},
		{"entity_resolve", `SELECT status,merged_into_entity_id FROM entity WHERE id=$1`,
			[]any{m9.EntityIDs[0]}},
	}
	out := make([]explain, 0, len(queries))
	for _, query := range queries {
		var plan json.RawMessage
		sql := "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) " + query.sql
		if err := pool.QueryRow(ctx, sql, query.args...).Scan(&plan); err != nil {
			return nil, fmt.Errorf("explain %s: %w", query.name, err)
		}
		out = append(out, explain{Name: query.name, Plan: plan})
	}
	return out, nil
}
