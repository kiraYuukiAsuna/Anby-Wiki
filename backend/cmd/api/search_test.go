package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/platform/httpx"
	wikisearch "github.com/anby/wiki/backend/internal/search"
)

// searchURL 构造搜索请求路径。
func searchURL(query string, extra ...string) string {
	u := "/api/v1/pages/search?q=" + url.QueryEscape(query)
	for _, e := range extra {
		u += "&" + e
	}
	return u
}

// createPage 仅创建页面（不发布），返回页面 ID。
func createPage(t *testing.T, router http.Handler, actorHeader map[string]string, title string) string {
	t.Helper()
	code, created := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": title}, actorHeader)
	if code != http.StatusCreated {
		t.Fatalf("创建页面 %q 失败: %d %v", title, code, created)
	}
	return created["id"].(string)
}

// searchItems 发起匿名搜索并断言 200。
func searchItems(t *testing.T, router http.Handler, path string) []any {
	t.Helper()
	code, body := doJSON(t, router, http.MethodGet, path, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	items, _ := body["items"].([]any)
	if items == nil {
		t.Fatalf("items 缺失或非数组: %v", body)
	}
	return items
}

func itemAt(t *testing.T, items []any, i int) map[string]any {
	t.Helper()
	m, _ := items[i].(map[string]any)
	if m == nil {
		t.Fatalf("items[%d] 非对象: %v", i, items[i])
	}
	return m
}

func TestSearchPagesRanking(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := map[string]string{"X-Actor-ID": tdb.MakeActor(t, "human", "alice").String()}
	exactID := createPage(t, router, actor, "Anby")
	prefixID := createPage(t, router, actor, "Anby Demara")
	containsID := createPage(t, router, actor, "Demara Anby")
	createPage(t, router, actor, "Billy Kid")

	items := searchItems(t, router, searchURL("  ANBY  "))
	if len(items) != 3 {
		t.Fatalf("命中数 = %d, want 3: %v", len(items), items)
	}
	want := []struct {
		id    string
		title string
	}{
		{exactID, "Anby"},
		{prefixID, "Anby Demara"},
		{containsID, "Demara Anby"},
	}
	for i, expected := range want {
		item := itemAt(t, items, i)
		if item["id"] != expected.id || item["display_title"] != expected.title {
			t.Fatalf("items[%d] = %v, want id=%s title=%q", i, item, expected.id, expected.title)
		}
		if item["matched_on"] != "title" || item["namespace"] != "main" {
			t.Fatalf("items[%d] matched_on/namespace 异常: %v", i, item)
		}
	}
}

func TestSearchPagesAliasHit(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actorHeader := map[string]string{"X-Actor-ID": tdb.MakeActor(t, "human", "alice").String()}
	pageID := createPage(t, router, actorHeader, "Old Name")

	code, _ := doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/pages/%s/rename", pageID),
		map[string]any{"title": "New Name"}, actorHeader)
	if code != http.StatusOK {
		t.Fatalf("改名失败: %d", code)
	}

	items := searchItems(t, router, searchURL("old name"))
	if len(items) != 1 {
		t.Fatalf("命中数 = %d, want 1: %v", len(items), items)
	}
	item := itemAt(t, items, 0)
	if item["id"] != pageID || item["display_title"] != "New Name" || item["matched_on"] != "alias" {
		t.Fatalf("别名命中结果异常: %v", item)
	}

	items = searchItems(t, router, searchURL("new name"))
	if len(items) != 1 || itemAt(t, items, 0)["matched_on"] != "title" {
		t.Fatalf("新标题应 title 命中: %v", items)
	}
}

func TestSearchPagesEmptyQuery(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actorHeader := map[string]string{"X-Actor-ID": tdb.MakeActor(t, "human", "alice").String()}
	createPage(t, router, actorHeader, "Some Page")

	for _, path := range []string{searchURL(""), searchURL("   "), "/api/v1/pages/search"} {
		items := searchItems(t, router, path)
		if len(items) != 0 {
			t.Fatalf("空 q 应返回空列表: %s -> %v", path, items)
		}
	}
}

func TestSearchPagesLimit(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actorHeader := map[string]string{"X-Actor-ID": tdb.MakeActor(t, "human", "alice").String()}
	for i := 1; i <= 3; i++ {
		createPage(t, router, actorHeader, fmt.Sprintf("Limit Page %d", i))
	}

	items := searchItems(t, router, searchURL("limit page", "limit=1"))
	if len(items) != 1 || itemAt(t, items, 0)["display_title"] != "Limit Page 1" {
		t.Fatalf("limit=1 结果异常: %v", items)
	}

	code, body := doJSON(t, router, http.MethodGet, searchURL("limit page", "limit=abc"), nil, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeBadRequest)
}

func TestSearchPagesEscapesLikeWildcards(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actorHeader := map[string]string{"X-Actor-ID": tdb.MakeActor(t, "human", "alice").String()}
	createPage(t, router, actorHeader, "100% Sure")
	createPage(t, router, actorHeader, "1000 Sure")

	items := searchItems(t, router, searchURL("100%"))
	if len(items) != 1 || itemAt(t, items, 0)["display_title"] != "100% Sure" {
		t.Fatalf("%% 应按字面匹配: %v", items)
	}
}

func TestSearchPagesExcludesSoftDeleted(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actorHeader := map[string]string{"X-Actor-ID": tdb.MakeActor(t, "human", "alice").String()}
	goneID := createPage(t, router, actorHeader, "Gone Search Page")
	createPage(t, router, actorHeader, "Gone Search Page Alive")
	tdb.SoftDeletePage(t, uuid.MustParse(goneID))

	items := searchItems(t, router, searchURL("gone search page"))
	if len(items) != 1 || itemAt(t, items, 0)["display_title"] != "Gone Search Page Alive" {
		t.Fatalf("软删除页面不应命中: %v", items)
	}
}

func TestSearchPagesNamespaceNotFound(t *testing.T) {
	_, router, _ := setupWriteAPI(t)
	code, body := doJSON(t, router, http.MethodGet, searchURL("x", "namespace=no-such"), nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)
}

type searchAdapterStub struct {
	query wikisearch.Query
	hits  []wikisearch.Hit
	total int
	calls int
}

func (s *searchAdapterStub) Index(context.Context, wikisearch.SearchDocument) error { return nil }
func (s *searchAdapterStub) Delete(context.Context, uuid.UUID) error                { return nil }
func (s *searchAdapterStub) Rebuild(context.Context, []wikisearch.SearchDocument) error {
	return nil
}
func (s *searchAdapterStub) Search(_ context.Context, query wikisearch.Query) ([]wikisearch.Hit, int, error) {
	s.query = query
	s.calls++
	return s.hits, s.total, nil
}

func TestSearchAPI_UsesSynchronousFallbackOnlyForTitleAliasFields(t *testing.T) {
	primary := &searchAdapterStub{}
	fallback := &searchAdapterStub{}
	api := NewSearchAPI(primary, uuid.MustParse("01910000-0000-7000-8000-000000000001")).
		WithTitleAliasFallback(fallback)

	for _, path := range []string{
		"/api/v1/pages/search?q=anby&fields=title&fields=alias",
		"/api/v1/pages/search?q=anby&fields=title,alias",
	} {
		api.searchPages(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	if fallback.calls != 2 || primary.calls != 0 {
		t.Fatalf("title/alias calls: primary=%d fallback=%d", primary.calls, fallback.calls)
	}

	api.searchPages(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/api/v1/pages/search?q=anby", nil))
	api.searchPages(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/api/v1/pages/search?q=anby&fields=body", nil))
	if primary.calls != 2 || fallback.calls != 2 {
		t.Fatalf("full-text calls: primary=%d fallback=%d", primary.calls, fallback.calls)
	}
}

func TestSearchAPI_MapsFiltersPaginationAndHighlight(t *testing.T) {
	pageID := uuid.MustParse("0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01")
	adapter := &searchAdapterStub{
		hits: []wikisearch.Hit{{
			PageID: pageID, DisplayTitle: "Anby Demara", Namespace: "main",
			MatchedOn: wikisearch.FieldBody, Highlight: "quiet [[swordswoman]]", Score: 2.5,
		}},
		total: 3,
	}
	api := NewSearchAPI(adapter, uuid.MustParse("01910000-0000-7000-8000-000000000001"))
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/pages/search?q=anby&namespace=main&language=zh-Hans&entity_type=character&fields=title&fields=body&limit=20&offset=2", nil)
	rec := httptest.NewRecorder()

	api.searchPages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if adapter.query.Language != "zh-Hans" || adapter.query.EntityType != "character" ||
		adapter.query.Limit != 20 || adapter.query.Offset != 2 ||
		len(adapter.query.Fields) != 2 || adapter.query.Fields[0] != wikisearch.FieldTitle ||
		adapter.query.Fields[1] != wikisearch.FieldBody {
		t.Fatalf("unexpected query: %+v", adapter.query)
	}
	var response pageSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 3 || len(response.Items) != 1 ||
		response.Items[0].Highlight != "quiet [[swordswoman]]" ||
		response.Items[0].MatchedOn != "body" || response.Items[0].Score != 2.5 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestSearchAPI_RejectsInvalidFieldAndPagination(t *testing.T) {
	api := NewSearchAPI(&searchAdapterStub{}, uuid.MustParse("01910000-0000-7000-8000-000000000001"))
	for _, path := range []string{
		"/api/v1/pages/search?q=anby&fields=secret",
		"/api/v1/pages/search?q=anby&limit=0",
		"/api/v1/pages/search?q=anby&limit=51",
		"/api/v1/pages/search?q=anby&offset=-1",
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			api.searchPages(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

var _ wikisearch.SearchAdapter = (*searchAdapterStub)(nil)
