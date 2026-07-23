package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	wikisearch "github.com/anby/wiki/backend/internal/search"
)

var (
	defaultWikiID      = uuid.MustParse("00000000-0000-7000-8000-000000000001")
	mainNamespaceID    = uuid.MustParse("00000000-0000-7000-8000-000000000101")
	defaultSystemActor = uuid.MustParse("00000000-0000-7000-8000-000000000201")
)

type fixture struct {
	PageID     uuid.UUID
	RevisionID uuid.UUID
	Title      string
}

func seed(ctx context.Context, pool *pgxpool.Pool, cfg config) ([]fixture, error) {
	pages := page.NewService(page.NewRepository(pool), db.NewTxManager(pool), id.NewGenerator())
	search := wikisearch.NewPostgresAdapter(pool)
	fixtures := make([]fixture, cfg.Pages)
	jobs := make(chan int)
	errs := make(chan error, cfg.Workers)
	var completed atomic.Int64
	var wg sync.WaitGroup
	for worker := 0; worker < cfg.Workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				item, err := seedPage(ctx, pages, search, index, cfg.Revisions)
				if err != nil {
					errs <- err
					return
				}
				fixtures[index] = item
				if n := completed.Add(1); n%10000 == 0 {
					fmt.Fprintf(os.Stderr, "seeded %d/%d pages\n", n, cfg.Pages)
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := 0; index < cfg.Pages; index++ {
			jobs <- index
		}
	}()
	wg.Wait()
	close(errs)
	if err := <-errs; err != nil {
		return nil, err
	}
	if err := seedRelations(ctx, pool, fixtures); err != nil {
		return nil, err
	}
	_, err := pool.Exec(ctx, `ANALYZE`)
	return fixtures, err
}

func seedPage(ctx context.Context, pages *page.Service, search *wikisearch.PostgresAdapter, index, revisions int) (fixture, error) {
	title := fmt.Sprintf("性能条目 %06d", index)
	p, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: defaultWikiID, NamespaceID: mainNamespaceID, Title: title, ActorID: defaultSystemActor,
	})
	if err != nil {
		return fixture{}, fmt.Errorf("create page %d: %w", index, err)
	}
	var revisionID uuid.UUID
	for revision := 0; revision < revisions; revision++ {
		var expected *uuid.UUID
		if revisionID != uuid.Nil {
			expected = &revisionID
		}
		rev, err := pages.Publish(ctx, page.PublishParams{
			PageID: p.ID, ActorID: defaultSystemActor, ExpectedRevisionID: expected,
			AST: astFor(index, revision), Summary: "performance fixture",
		})
		if err != nil {
			return fixture{}, fmt.Errorf("publish page %d revision %d: %w", index, revision, err)
		}
		revisionID = rev.ID
	}
	err = search.Index(ctx, wikisearch.SearchDocument{
		PageID: p.ID, WikiID: defaultWikiID, Namespace: "main", Language: "zh-Hans",
		SourceRevisionID: revisionID, DisplayTitle: title, NormalizedTitle: title,
		Aliases: []string{fmt.Sprintf("别名 %06d", index)},
		Body:    fmt.Sprintf("共同基准词 postgres fts 容量测试正文 分片 %03d", index%1000),
	})
	return fixture{PageID: p.ID, RevisionID: revisionID, Title: title}, err
}

func astFor(pageIndex, revision int) json.RawMessage {
	blockID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("perf-%d-%d", pageIndex, revision)))
	return json.RawMessage(fmt.Sprintf(
		`{"type":"document","schema_version":1,"children":[{"id":%q,"type":"paragraph","content":[{"type":"text","text":%q}]}]}`,
		blockID, fmt.Sprintf("共同基准词 页面 %d 修订 %d", pageIndex, revision)))
}

func seedRelations(ctx context.Context, pool *pgxpool.Pool, fixtures []fixture) error {
	rows := make([][]any, 0, len(fixtures))
	for index, source := range fixtures {
		target := fixtures[index%min(100, len(fixtures))]
		rows = append(rows, []any{
			source.PageID, source.RevisionID,
			uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("link-%d", index))),
			"0", target.PageID, "resolved", target.Title,
		})
	}
	_, err := pool.CopyFrom(ctx, pgx.Identifier{"page_link_projection"},
		[]string{"source_page_id", "source_revision_id", "source_block_id", "source_node_id",
			"target_page_id", "resolution_status", "display_text"},
		pgx.CopyFromRows(rows))
	return err
}
