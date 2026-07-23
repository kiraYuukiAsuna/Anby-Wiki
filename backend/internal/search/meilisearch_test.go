package search

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMeilisearchAdapter_ProtocolSettingsIndexSearchDeleteAndIdempotency(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var indexBodies [][]byte
	settingsSeen := false
	searchSeen := false
	deleteSeen := false
	indexExists := false
	taskUID := int64(0)
	secret := "test-master-key"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Errorf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/indexes/pages":
			if !indexExists {
				writeTestJSON(w, http.StatusNotFound, map[string]string{"code": "index_not_found"})
				return
			}
			writeTestJSON(w, http.StatusOK, map[string]string{"uid": "pages"})
		case r.Method == http.MethodPost && r.URL.Path == "/indexes":
			indexExists = true
			taskUID++
			writeTestJSON(w, http.StatusAccepted, map[string]any{"taskUid": taskUID})
		case r.Method == http.MethodPatch && r.URL.Path == "/indexes/pages/settings":
			var body map[string]any
			decodeTestJSON(t, r.Body, &body)
			settingsSeen = containsAny(body["filterableAttributes"], "wiki_id", "namespace", "language", "entity_type") &&
				containsAny(body["searchableAttributes"], "display_title", "aliases", "body", "entity_terms")
			taskUID++
			writeTestJSON(w, http.StatusAccepted, map[string]any{"taskUid": taskUID})
		case r.Method == http.MethodPut && r.URL.Path == "/indexes/pages/documents":
			if r.URL.Query().Get("primaryKey") != "page_id" {
				t.Errorf("primaryKey = %q", r.URL.Query().Get("primaryKey"))
			}
			raw, _ := io.ReadAll(r.Body)
			mu.Lock()
			indexBodies = append(indexBodies, raw)
			mu.Unlock()
			taskUID++
			writeTestJSON(w, http.StatusAccepted, map[string]any{"taskUid": taskUID})
		case r.Method == http.MethodPost && r.URL.Path == "/indexes/pages/search":
			var body map[string]any
			decodeTestJSON(t, r.Body, &body)
			searchSeen = body["q"] == "needle" &&
				int(body["limit"].(float64)) == 7 &&
				int(body["offset"].(float64)) == 3 &&
				body["highlightPreTag"] == "[[" &&
				strings.Contains(body["filter"].(string), `namespace = "main"`) &&
				containsAny(body["attributesToSearchOn"], "body")
			writeTestJSON(w, http.StatusOK, map[string]any{
				"estimatedTotalHits": 1,
				"hits": []any{map[string]any{
					"page_id":       uuid.MustParse("0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4001").String(),
					"display_title": "Needle Page",
					"namespace":     "main",
					"_rankingScore": 0.91,
					"_formatted": map[string]any{
						"body": "text [[needle]] text",
					},
				}},
			})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/indexes/pages/documents/"):
			deleteSeen = true
			taskUID++
			writeTestJSON(w, http.StatusAccepted, map[string]any{"taskUid": taskUID})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/tasks/"):
			writeTestJSON(w, http.StatusOK, map[string]string{"status": "succeeded"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			writeTestJSON(w, http.StatusNotFound, map[string]string{"code": "not_found"})
		}
	}))
	defer server.Close()

	adapter := newTestMeiliAdapter(t, server.URL, secret)
	ctx := context.Background()
	if err := adapter.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	doc := validMeiliTestDocument()
	if err := adapter.Index(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Index(ctx, doc); err != nil {
		t.Fatal(err)
	}
	hits, total, err := adapter.Search(ctx, Query{
		Text: "needle", WikiID: doc.WikiID, Namespace: "main",
		Fields: []Field{FieldBody}, Limit: 7, Offset: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(hits) != 1 || hits[0].MatchedOn != FieldBody ||
		hits[0].Highlight != "text [[needle]] text" || hits[0].Score != 0.91 {
		t.Fatalf("unexpected hits: total=%d hits=%+v", total, hits)
	}
	if err := adapter.Delete(ctx, doc.PageID); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !settingsSeen || !searchSeen || !deleteSeen {
		t.Fatalf("protocol coverage: settings=%v search=%v delete=%v", settingsSeen, searchSeen, deleteSeen)
	}
	if len(indexBodies) != 2 || string(indexBodies[0]) != string(indexBodies[1]) {
		t.Fatalf("duplicate Index must be idempotent: requests=%d", len(indexBodies))
	}
	if strings.Contains(string(indexBodies[0]), secret) {
		t.Fatal("API key leaked into document payload")
	}
}

func TestMeilisearchAdapter_TaskFailureIsRetryableAndDoesNotLeakKey(t *testing.T) {
	t.Parallel()
	secret := "never-log-this-key"
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			attempts++
			writeTestJSON(w, http.StatusAccepted, map[string]any{"taskUid": attempts})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/tasks/"):
			if attempts == 1 {
				writeTestJSON(w, http.StatusOK, map[string]any{
					"status": "failed",
					"error": map[string]string{
						"code": "internal", "message": "failed with " + secret,
					},
				})
				return
			}
			writeTestJSON(w, http.StatusOK, map[string]string{"status": "succeeded"})
		default:
			writeTestJSON(w, http.StatusNotFound, map[string]string{"code": "not_found"})
		}
	}))
	defer server.Close()

	adapter := newTestMeiliAdapter(t, server.URL, secret)
	err := adapter.Index(context.Background(), validMeiliTestDocument())
	if err == nil || !strings.Contains(err.Error(), "internal") {
		t.Fatalf("first Index should expose task code, err=%v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("API key leaked through task error")
	}
	if err := adapter.Index(context.Background(), validMeiliTestDocument()); err != nil {
		t.Fatalf("retry should succeed: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d, want 2", attempts)
	}
}

func TestMeilisearchAdapter_EmptyQueryDoesNotCallRemote(t *testing.T) {
	t.Parallel()
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()
	adapter := newTestMeiliAdapter(t, server.URL, "")
	hits, total, err := adapter.Search(context.Background(), Query{Text: "  "})
	if err != nil || total != 0 || len(hits) != 0 || called {
		t.Fatalf("empty query = (%+v,%d,%v), called=%v", hits, total, err, called)
	}
}

func newTestMeiliAdapter(t *testing.T, baseURL, key string) *MeilisearchAdapter {
	t.Helper()
	adapter, err := NewMeilisearchAdapter(MeilisearchConfig{
		BaseURL: baseURL, APIKey: key, Index: "pages",
		HTTPClient: &http.Client{Timeout: time.Second}, TaskPollInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func validMeiliTestDocument() SearchDocument {
	return SearchDocument{
		PageID:           uuid.MustParse("0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4001"),
		WikiID:           uuid.MustParse("0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4002"),
		SourceRevisionID: uuid.MustParse("0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4003"),
		Namespace:        "main", Language: "zh-CN", DisplayTitle: "Needle Page",
		NormalizedTitle: "needle-page", Aliases: []string{"needle"},
		Body: "text needle text", EntityTerms: []string{"entity"},
	}
}

func decodeTestJSON(t *testing.T, body io.Reader, target any) {
	t.Helper()
	if err := json.NewDecoder(body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func writeTestJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func containsAny(raw any, values ...string) bool {
	items, ok := raw.([]any)
	if !ok {
		return false
	}
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item.(string)] = true
	}
	for _, value := range values {
		if !set[value] {
			return false
		}
	}
	return true
}
