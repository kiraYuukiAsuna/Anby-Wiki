package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReferenceSearchValidation(t *testing.T) {
	for _, query := range []string{"cursor=bad", "q=" + url.QueryEscape(strings.Repeat("字", 256)), "page_size=0", "page_size=101"} {
		response := httptest.NewRecorder()
		_, _, _, ok := referenceSearchParams(response, httptest.NewRequest(http.MethodGet, "/?"+query, nil))
		if ok || response.Code != http.StatusBadRequest {
			t.Fatalf("accepted %s: %d", query, response.Code)
		}
	}
}

// A disposable schema exercises the SQL, pagination and authorization without production data.
func TestReferenceOptionsE2E(t *testing.T) {
	databaseURL := os.Getenv("PAGE_E2E_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set PAGE_E2E_DATABASE_URL to test reference search")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schema := "reference_options_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE")
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path TO "+quoted)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `
 CREATE TABLE actor(id uuid PRIMARY KEY,display_name text,status text,actor_type text,wiki_id uuid,external_key text,user_id uuid);
 CREATE TABLE local_account(id uuid,actor_id uuid,username text,status text);
 CREATE TABLE role(id uuid,role_key text);
 CREATE TABLE actor_role(actor_id uuid,wiki_id uuid,role_id uuid);
 CREATE TABLE entity(id uuid,wiki_id uuid,canonical_key text,status text);
 CREATE TABLE entity_label(entity_id uuid,label text,is_primary boolean,language text);
 CREATE TABLE property(id uuid,name text);
 CREATE TABLE claim(id uuid,subject_entity_id uuid,property_id uuid,target_entity_id uuid,value_json jsonb,status text);
 CREATE TABLE source(id uuid,wiki_id uuid,title text);
 CREATE TABLE source_version(id uuid,source_id uuid);
 CREATE TABLE citation(id uuid,source_version_id uuid,quotation text,locator_json jsonb);
 `)
	if err != nil {
		t.Fatal(err)
	}
	wiki, other, actor, guest, entity, property, source, version := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO actor VALUES($1,'编辑甲','active','human',$2,'',NULL),($3,'外部访客','active','human',$4,'',NULL)`, actor, wiki, guest, other)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO actor_role(actor_id,wiki_id) VALUES($1,$2),($3,$4)`, actor, wiki, guest, other)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO entity VALUES($1,$2,'entity-one','active');`, entity, wiki)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO entity_label VALUES($1,'搜索实体',true,'zh')`, entity)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO property VALUES($1,'作者')`, property)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO source VALUES($1,$2,'搜索来源')`, source, wiki)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO source_version VALUES($1,$2)`, version, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"published", "published", "proposed", "deprecated"} {
		_, err = pool.Exec(ctx, `INSERT INTO claim VALUES($1,$2,$3,NULL,'"事实值"',$4)`, uuid.New(), entity, property, status)
		if err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		_, err = pool.Exec(ctx, `INSERT INTO citation VALUES($1,$2,'引用原文','{}')`, uuid.New(), version)
		if err != nil {
			t.Fatal(err)
		}
	}
	auth := governance.NewAuthorizationService(pool)
	api := NewKnowledgeReadAPI(knowledge.NewService(knowledge.NewRepository(pool), nil, nil, nil), evidence.NewService(evidence.NewRepository(pool), nil, nil, "test", nil, nil)).WithMergeAuthorization(auth)
	memberAPI := &GovernanceAPI{auth: auth, userAdmin: governance.NewUserAdminService(governance.NewRepository(pool), auth, nil, nil)}
	type page struct {
		Items []struct {
			ID          string `json:"id"`
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"items"`
		Next string `json:"next_cursor"`
	}
	for _, tc := range []struct {
		name, query string
		handler     http.HandlerFunc
		count       int
	}{{"claims", "搜索实体", api.listClaimOptions, 2}, {"citations", "引用原文", api.listCitationOptions, 2}, {"members", "编辑", memberAPI.listMemberOptions, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			request := func(who, where uuid.UUID, query string) (int, []byte) {
				r := httptest.NewRequest(http.MethodGet, "/?"+query, nil)
				r = r.WithContext(authdomain.WithPrincipal(ctx, authdomain.Principal{ActorID: who, WikiID: where}))
				w := httptest.NewRecorder()
				tc.handler(w, r)
				return w.Code, w.Body.Bytes()
			}
			cursor := ""
			seen := map[string]bool{}
			for i := 0; i < 3; i++ {
				code, body := request(actor, wiki, "page_size=1&q="+url.QueryEscape(tc.query)+"&cursor="+cursor)
				if code != 200 {
					t.Fatalf("%d %s", code, body)
				}
				var result page
				if err := json.Unmarshal(body, &result); err != nil {
					t.Fatal(err)
				}
				for _, item := range result.Items {
					if seen[item.ID] || item.Label == "" {
						t.Fatalf("invalid choice %#v", item)
					}
					seen[item.ID] = true
				}
				cursor = result.Next
				if cursor == "" {
					break
				}
			}
			if len(seen) != tc.count {
				t.Fatalf("got %d choices, want %d", len(seen), tc.count)
			}
			code, body := request(guest, wiki, "")
			if code != 403 {
				t.Fatalf("guest read catalog: %d %s", code, body)
			}
			code, body = request(guest, other, "q="+url.QueryEscape(tc.query))
			var result page
			json.Unmarshal(body, &result)
			if code != 200 || len(result.Items) != 0 {
				t.Fatalf("foreign Wiki leaked results: %d %s", code, body)
			}
			code, body = request(actor, wiki, "q=%25")
			json.Unmarshal(body, &result)
			if code != 200 || len(result.Items) != 0 {
				t.Fatalf("wildcard query matched names: %d %s", code, body)
			}
			if strings.Contains(string(body), "email") {
				t.Fatal("catalog exposed account private fields")
			}
		})
	}
}
