package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

func TestPageCatalogInvalidPage(t *testing.T) {
	api := NewReadAPI(nil)
	for _, query := range []string{
		"page=", "page=0", "page=-1", "page=1.5", "page=abc",
		"page=2147483648", "page=999999999999999999999", "page=1&cursor=anything",
	} {
		t.Run(query, func(t *testing.T) {
			response := httptest.NewRecorder()
			api.listPages(response, httptest.NewRequest(http.MethodGet, "/api/v1/pages?"+query, nil))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
		})
	}
}

// Each run owns an isolated schema; no existing Wiki data is read or modified.
func TestPageCatalogPaginationE2E(t *testing.T) {
	databaseURL := os.Getenv("PAGE_E2E_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set PAGE_E2E_DATABASE_URL to test numbered pagination and share filtering")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schemaName := "page_catalog_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schemaSQL := pgx.Identifier{schemaName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schemaSQL); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schemaSQL+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schemaName + ",public"
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path TO "+schemaSQL+",public")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema, err := os.ReadFile("../../migrations/000001_initial_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
	wikiID, ownerID, guestID, namespaceID, otherNamespaceID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	txm := db.NewTxManager(pool)
	// Authorization and namespace fixtures; Page creation/publication uses the domain service.
	err = txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO wiki_site
			(id,site_key,name,default_language,owner_actor_id)
			VALUES ($1,$2,'Catalog test','en',$3)`, wikiID, wikiID.String(), ownerID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO actor (id,wiki_id,actor_type,display_name)
			VALUES ($1,$3,'human','Owner'),($2,$3,'human','Guest')`, ownerID, guestID, wikiID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO actor_role (actor_id,wiki_id,role_id)
			SELECT $1,$2,id FROM role WHERE role_key='admin'`, ownerID, wikiID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO namespace
			(id,wiki_id,namespace_key,canonical_name,display_name,is_content)
			VALUES ($1,$3,'main','Main','Main',true),($2,$3,'talk','Talk','Talk',false)`,
			namespaceID, otherNamespaceID, wikiID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	pages := page.NewService(page.NewRepository(pool), txm, id.NewGenerator())
	api := NewReadAPI(pages).WithAuthorization(governance.NewAuthorizationService(pool))
	expected := map[string][]*page.Page{}
	shared := map[string][]*page.Page{}
	for _, status := range []string{page.CatalogStatusPublished, page.CatalogStatusDraft} {
		for index := 0; index < 7; index++ {
			value, err := pages.CreatePage(ctx, page.CreatePageParams{
				WikiID: wikiID, NamespaceID: namespaceID, ActorID: ownerID,
				Title: fmt.Sprintf("%s %02d", status, 6-index),
			})
			if err != nil {
				t.Fatal(err)
			}
			if status == page.CatalogStatusPublished {
				if _, err := pages.Publish(ctx, page.PublishParams{
					PageID: value.ID, ActorID: ownerID,
					AST: json.RawMessage(`{"schema_version":1,"type":"document","children":[]}`),
				}); err != nil {
					t.Fatal(err)
				}
				value, err = pages.GetPage(ctx, value.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			expected[status] = append(expected[status], value)
			if index%2 == 0 {
				shared[status] = append(shared[status], value)
				if _, err := pool.Exec(ctx, `INSERT INTO page_share (page_id,actor_id,role_key,granted_by)
					VALUES ($1,$2,$3,$4)`, value.ID, guestID, governance.PageShareRoleViewer, ownerID); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if _, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: wikiID, NamespaceID: otherNamespaceID, ActorID: ownerID, Title: "Hidden talk page",
	}); err != nil {
		t.Fatal(err)
	}
	read := func(actorID uuid.UUID, query string) pageCatalogResponse {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/pages?"+query, nil)
		request = request.WithContext(authdomain.WithPrincipal(ctx, authdomain.Principal{ActorID: actorID, WikiID: wikiID}))
		response := httptest.NewRecorder()
		api.listPages(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("query=%s status=%d body=%s", query, response.Code, response.Body)
		}
		var result pageCatalogResponse
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	for _, actorID := range []uuid.UUID{ownerID, guestID} {
		for _, status := range []string{page.CatalogStatusPublished, page.CatalogStatusDraft} {
			for _, sortKey := range []string{page.CatalogSortTitle, page.CatalogSortRecent} {
				want := slices.Clone(expected[status])
				if actorID == guestID {
					want = slices.Clone(shared[status])
				}
				slices.SortFunc(want, func(a, b *page.Page) int {
					if sortKey == page.CatalogSortTitle {
						if cmp := strings.Compare(a.NormalizedTitle, b.NormalizedTitle); cmp != 0 {
							return cmp
						}
						return strings.Compare(a.ID.String(), b.ID.String())
					}
					if cmp := b.UpdatedAt.Compare(a.UpdatedAt); cmp != 0 {
						return cmp
					}
					return strings.Compare(b.ID.String(), a.ID.String())
				})
				query := fmt.Sprintf("status=%s&sort=%s&page_size=3", status, sortKey)
				for _, number := range []int{2, 1, 3, page.MaxCatalogPageNumber} {
					result := read(actorID, fmt.Sprintf("%s&page=%d", query, number))
					start := min((number-1)*3, len(want))
					end := min(start+3, len(want))
					if result.Total != len(want) || len(result.Items) != end-start {
						t.Fatalf("actor=%s %s page=%d: got %d/%d want %d/%d", actorID, query, number,
							len(result.Items), result.Total, end-start, len(want))
					}
					for index, item := range result.Items {
						if item.ID != want[start+index].ID {
							t.Fatalf("%s page=%d item=%d: got %s want %s", query, number, index, item.ID, want[start+index].ID)
						}
					}
				}
				first := read(actorID, query)
				second := read(actorID, query+"&cursor="+url.QueryEscape(*first.NextCursor))
				if second.Total != len(want) || second.Items[0].ID != want[3].ID {
					t.Fatalf("cursor compatibility failed: %#v", second)
				}
			}
		}
	}
}
