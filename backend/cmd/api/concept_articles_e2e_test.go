package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAPIConceptArticlesE2E(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("API_E2E_BASE_URL"), "/")
	password := os.Getenv("API_E2E_PASSWORD")
	if baseURL == "" || password == "" {
		t.Skip("set API_E2E_BASE_URL and API_E2E_PASSWORD")
	}
	runID := strconv.FormatInt(time.Now().UnixNano(), 36)
	admin, _ := apiE2EAdmin(t, baseURL, password)
	session := requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/auth/session", nil, http.StatusOK)
	wikiID := e2eUUID(t, session, "wiki_id")
	entityID, _, _ := createEntityThroughProposalE2E(t, admin, admin, baseURL, wikiID, runID)
	endpoint := fmt.Sprintf("%s/api/v1/entities/%s/articles?language=zh-Hans", baseURL, entityID)
	assertArticles := func(client *http.Client, headers http.Header, want []uuid.UUID) {
		t.Helper()
		result := requestE2EMapHeaders(t, client, http.MethodGet, endpoint, nil, headers, http.StatusOK)
		rawItems, ok := result["items"].([]any)
		if !ok || len(rawItems) != len(want) {
			t.Fatalf("articles=%#v want=%v", result, want)
		}
		for i, raw := range rawItems {
			item, ok := raw.(map[string]any)
			if !ok || e2eUUID(t, item, "page_id") != want[i] {
				t.Fatalf("article[%d]=%#v want=%s", i, raw, want[i])
			}
		}
	}
	assertArticles(admin, nil, nil)

	createPage := func(title, language, role string, published bool) uuid.UUID {
		t.Helper()
		result := requestE2EMap(t, admin, http.MethodPost, baseURL+"/api/v1/pages",
			map[string]any{"namespace": "main", "title": title + " " + runID, "language": language},
			http.StatusCreated)
		pageID := e2eUUID(t, result, "id")
		requestE2EMap(t, admin, http.MethodPut,
			fmt.Sprintf("%s/api/v1/pages/%s/entity-bindings", baseURL, pageID),
			map[string]any{"entity_id": entityID, "role": role, "language": language}, http.StatusOK)
		if published {
			doc := documentE2E(uuid.New(), title, "concept destination")
			requestE2EMap(t, admin, http.MethodPost,
				fmt.Sprintf("%s/api/v1/pages/%s/revisions", baseURL, pageID),
				map[string]any{"ast": doc, "summary": "concept article fixture", "is_minor": false},
				http.StatusCreated)
		}
		return pageID
	}
	first := createPage("Zulu concept", "zh-Hans", "primary", true)
	assertArticles(admin, nil, []uuid.UUID{first})
	second := createPage("Alpha concept", "en", "primary", true)
	createPage("Mention only", "zh-Hans", "mentioned", true)
	createPage("Unpublished concept", "zh-Hans", "primary", false)
	redirectPage := createPage("Redirect concept", "zh-Hans", "primary", true)
	requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/pages/%s/redirect", baseURL, redirectPage),
		map[string]any{"target": map[string]any{"kind": "page", "target_page_id": first}},
		http.StatusOK)
	assertArticles(admin, nil, []uuid.UUID{first, second})

	username := "concept_guest_" + runID
	guest, guestActorID := registerE2EUser(t, baseURL, username, username+"@example.invalid", password)
	// Foreign-Wiki identities are indistinguishable from missing identities.
	requestE2EMap(t, guest, http.MethodGet, endpoint, nil, http.StatusNotFound)
	requestE2EMap(t, admin, http.MethodPut,
		fmt.Sprintf("%s/api/v1/pages/%s/shares", baseURL, first),
		map[string]any{"identifier": username, "role_key": "viewer"}, http.StatusOK)
	headers := http.Header{"X-Wiki-ID": []string{wikiID.String()}}
	assertArticles(guest, headers, []uuid.UUID{first})
	// A guest can see this Wiki via another shared page, but no destination.
	sharedMention := createPage("Guest source", "zh-Hans", "mentioned", true)
	requestE2EMap(t, admin, http.MethodPut,
		fmt.Sprintf("%s/api/v1/pages/%s/shares", baseURL, sharedMention),
		map[string]any{"identifier": username, "role_key": "viewer"}, http.StatusOK)
	requestE2EMap(t, admin, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/pages/%s/shares/%s", baseURL, first, guestActorID),
		nil, http.StatusOK)
	assertArticles(guest, headers, nil)

	mergedID, _, _ := createEntityThroughProposalE2E(t, admin, admin, baseURL, wikiID, runID+"-old")
	oldPage := createPage("Original identity article", "en", "primary", true)
	requestE2EMap(t, admin, http.MethodPut,
		fmt.Sprintf("%s/api/v1/pages/%s/entity-bindings", baseURL, oldPage),
		map[string]any{"entity_id": mergedID, "role": "primary", "language": "en"}, http.StatusOK)
	requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/entities/%s/merge", baseURL, mergedID),
		map[string]any{"target_entity_id": entityID, "reason": "concept resolution fixture"},
		http.StatusOK)
	merged := requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/entities/%s/articles", baseURL, mergedID), nil, http.StatusOK)
	if e2eUUID(t, merged, "entity_id") != entityID || len(merged["items"].([]any)) != 3 {
		t.Fatalf("merged concept failed to resolve: %#v", merged)
	}
	assertArticles(admin, nil, []uuid.UUID{first, second, oldPage})
}
