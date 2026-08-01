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

func TestMeilisearchAdapter_ProtocolHybridFacetsAndIdempotency(t *testing.T) {
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
			embedders, ok := body["embedders"].(map[string]any)
			settingsSeen = ok &&
				containsAny(body["filterableAttributes"], "wiki_id", "namespace", "language", "entity_type") &&
				containsAny(body["searchableAttributes"], "display_title", "aliases", "body", "entity_terms") &&
				embedders["page-content"] != nil
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
			hybrid, ok := body["hybrid"].(map[string]any)
			searchSeen = ok && body["q"] == "needle" &&
				int(body["limit"].(float64)) == 7 &&
				int(body["offset"].(float64)) == 3 &&
				hybrid["embedder"] == "page-content" &&
				hybrid["semanticRatio"].(float64) == 0.65 &&
				strings.Contains(body["filter"].(string), `namespace = "main"`) &&
				containsAny(body["facets"], "namespace", "language", "entity_type")
			writeTestJSON(w, http.StatusOK, map[string]any{
				"estimatedTotalHits": 1,
				"facetDistribution": map[string]any{
					"namespace":   map[string]int{"main": 1},
					"language":    map[string]int{"zh-CN": 1},
					"entity_type": map[string]int{"character": 1},
				},
				"hits": []any{map[string]any{
					"page_id":       uuid.MustParse("0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4001").String(),
					"display_title": "Needle Page",
					"namespace":     "main",
					"language":      "zh-CN",
					"entity_id":     uuid.MustParse("0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4004").String(),
					"entity_type":   "character",
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

	adapter := newTestMeiliAdapter(t, server.URL, secret, true)
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
	result, err := adapter.Search(ctx, Query{
		Text: "needle", WikiID: doc.WikiID, Namespace: "main",
		Fields: []Field{FieldBody}, Mode: ModeHybrid, SemanticRatio: 0.65,
		Limit: 7, Offset: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Hits) != 1 || result.Mode != ModeHybrid ||
		result.Hits[0].MatchedOn != FieldBody ||
		result.Hits[0].Highlight != "text [[needle]] text" ||
		result.Hits[0].Score != 0.91 ||
		len(result.Facets.EntityTypes) != 1 {
		t.Fatalf("unexpected result: %+v", result)
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

func TestMeilisearchAdapter_SemanticModeRequiresEmbedder(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("semantic query without embedder must not call remote")
	}))
	defer server.Close()
	adapter := newTestMeiliAdapter(t, server.URL, "", false)
	_, err := adapter.Search(context.Background(), Query{Text: "meaning", Mode: ModeSemantic})
	if err != ErrSemanticUnavailable {
		t.Fatalf("err = %v, want ErrSemanticUnavailable", err)
	}
	if modes := adapter.Capabilities().Modes; len(modes) != 1 || modes[0] != ModeKeyword {
		t.Fatalf("modes = %v", modes)
	}
}

func TestMeilisearchAdapter_TaskFailureDoesNotLeakKey(t *testing.T) {
	t.Parallel()
	secret := "never-log-this-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			writeTestJSON(w, http.StatusAccepted, map[string]any{"taskUid": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/tasks/1":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"status": "failed",
				"error":  map[string]string{"code": "internal", "message": "failed with " + secret},
			})
		default:
			writeTestJSON(w, http.StatusNotFound, map[string]string{"code": "not_found"})
		}
	}))
	defer server.Close()
	adapter := newTestMeiliAdapter(t, server.URL, secret, false)
	err := adapter.Index(context.Background(), validMeiliTestDocument())
	if err == nil || !strings.Contains(err.Error(), "internal") || strings.Contains(err.Error(), secret) {
		t.Fatalf("unexpected sanitized error: %v", err)
	}
}

func newTestMeiliAdapter(t *testing.T, baseURL, key string, semantic bool) *MeilisearchAdapter {
	t.Helper()
	var embedder string
	var config map[string]any
	if semantic {
		embedder = "page-content"
		config = map[string]any{
			"source": "huggingFace",
			"model":  "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2",
		}
	}
	adapter, err := NewMeilisearchAdapter(MeilisearchConfig{
		BaseURL: baseURL, APIKey: key, Index: "pages",
		HTTPClient: &http.Client{Timeout: time.Second}, TaskPollInterval: time.Millisecond,
		TaskTimeout: time.Second, SemanticEmbedder: embedder, EmbedderConfig: config,
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
