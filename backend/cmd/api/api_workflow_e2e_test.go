package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAPICoreWorkflowsE2E(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("API_E2E_BASE_URL"), "/")
	password := os.Getenv("API_E2E_PASSWORD")
	if baseURL == "" || password == "" {
		t.Skip("set API_E2E_BASE_URL and API_E2E_PASSWORD")
	}
	runID := strconv.FormatInt(time.Now().UnixNano(), 36)
	client, _ := apiE2EAdmin(t, baseURL, password)

	t.Run("pages history redirects and projections", func(t *testing.T) {
		testPageWorkflow(t, client, baseURL, runID)
	})
	t.Run("assets", func(t *testing.T) {
		testAssetWorkflow(t, client, baseURL, runID)
	})
	t.Run("sources", func(t *testing.T) {
		testSourceWorkflow(t, client, baseURL, runID)
	})
	t.Run("datasets", func(t *testing.T) {
		testDatasetWorkflow(t, client, baseURL, runID)
	})
	t.Run("components", func(t *testing.T) {
		testComponentWorkflow(t, client, baseURL, runID)
	})
	t.Run("collections", func(t *testing.T) {
		testCollectionWorkflow(t, client, baseURL, runID)
	})
}

func TestAPICLIAuthE2E(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("API_E2E_BASE_URL"), "/")
	password := os.Getenv("API_E2E_PASSWORD")
	if baseURL == "" || password == "" {
		t.Skip("set API_E2E_BASE_URL and API_E2E_PASSWORD")
	}
	admin, adminActor := apiE2EAdmin(t, baseURL, password)
	codeResponse := requestE2EMap(t, admin, http.MethodPost,
		baseURL+"/api/v1/auth/cli/codes", map[string]any{
			"name": "API E2E Agent", "token_ttl_days": 30,
		}, http.StatusCreated)
	code, ok := codeResponse["code"].(string)
	if !ok || !strings.HasPrefix(code, "anby_code_") {
		t.Fatalf("invalid CLI authorization code response: %#v", codeResponse)
	}

	anonymous := &http.Client{Timeout: collaborationE2ETimeout}
	exchange := requestE2EMap(t, anonymous, http.MethodPost,
		baseURL+"/api/v1/auth/cli/exchange",
		map[string]any{"code": code}, http.StatusOK)
	token, ok := exchange["token"].(string)
	if !ok || !strings.HasPrefix(token, "anby_token_") {
		t.Fatalf("invalid CLI token response: %#v", exchange)
	}
	if e2eUUID(t, exchange, "actor_id") != adminActor {
		t.Fatalf("CLI token actor=%v want=%s", exchange["actor_id"], adminActor)
	}
	tokenInfo, ok := exchange["token_info"].(map[string]any)
	if !ok {
		t.Fatalf("CLI exchange has no token_info: %#v", exchange)
	}
	tokenID := e2eUUID(t, tokenInfo, "id")

	bearer := &http.Client{Timeout: collaborationE2ETimeout}
	sessionRequest, err := http.NewRequest(
		http.MethodGet, baseURL+"/api/v1/auth/session", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	sessionRequest.Header.Set("Authorization", "Bearer "+token)
	sessionResponse, err := bearer.Do(sessionRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer sessionResponse.Body.Close()
	if sessionResponse.StatusCode != http.StatusOK {
		t.Fatalf("CLI session status=%d: %s",
			sessionResponse.StatusCode, readE2EBody(sessionResponse.Body))
	}
	var session map[string]any
	if err := json.NewDecoder(sessionResponse.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session["method"] != "cli_token" {
		t.Fatalf("CLI auth method=%v want=cli_token", session["method"])
	}

	codeRequest := doE2ERequest(t, bearer, http.MethodPost,
		baseURL+"/api/v1/auth/cli/codes",
		map[string]any{"name": "nested", "token_ttl_days": 30},
		http.Header{"Authorization": []string{"Bearer " + token}})
	assertE2EStatus(t, codeRequest, http.StatusForbidden)

	tokens := requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/auth/cli/tokens", nil, http.StatusOK)
	items, ok := tokens["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("CLI token list is empty: %#v", tokens)
	}
	requestE2ENoBody(t, admin, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/auth/cli/tokens/%s", baseURL, tokenID),
		http.StatusNoContent)

	revokedRequest, err := http.NewRequest(
		http.MethodGet, baseURL+"/api/v1/auth/session", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	revokedRequest.Header.Set("Authorization", "Bearer "+token)
	revokedResponse, err := bearer.Do(revokedRequest)
	if err != nil {
		t.Fatal(err)
	}
	assertE2EStatus(t, revokedResponse, http.StatusUnauthorized)
	requestE2EMap(t, anonymous, http.MethodPost,
		baseURL+"/api/v1/auth/cli/exchange",
		map[string]any{"code": code}, http.StatusUnauthorized)
	requestE2ENoBody(t, admin, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/auth/cli/token-records/%s", baseURL, tokenID),
		http.StatusNoContent)
	deleted := requestE2EMap(t, admin, http.MethodDelete,
		baseURL+"/api/v1/auth/cli/token-records", nil, http.StatusOK)
	if _, ok := deleted["deleted"].(float64); !ok {
		t.Fatalf("inactive token cleanup response lacks deleted count: %#v", deleted)
	}
}

func TestAPIGovernanceKnowledgeE2E(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("API_E2E_BASE_URL"), "/")
	password := os.Getenv("API_E2E_PASSWORD")
	if baseURL == "" || password == "" {
		t.Skip("set API_E2E_BASE_URL and API_E2E_PASSWORD")
	}
	runID := strconv.FormatInt(time.Now().UnixNano(), 36)
	admin, adminActor := apiE2EAdmin(t, baseURL, password)
	editor, editorActor := registerE2EUser(
		t, baseURL, "gov_editor_"+runID,
		"gov_editor_"+runID+"@example.invalid", password,
	)

	users := requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/admin/users?search=gov_editor_"+runID,
		nil, http.StatusOK)
	items, ok := users["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("admin user list did not find editor: %#v", users)
	}
	granted := requestE2EMap(t, admin, http.MethodPut,
		fmt.Sprintf("%s/api/v1/admin/users/%s/roles/reviewer", baseURL, editorActor),
		nil, http.StatusOK)
	if changed, _ := granted["changed"].(bool); !changed {
		t.Fatalf("grant reviewer was not reported as changed: %#v", granted)
	}
	revoked := requestE2EMap(t, admin, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/admin/users/%s/roles/reviewer", baseURL, editorActor),
		nil, http.StatusOK)
	if changed, _ := revoked["changed"].(bool); !changed {
		t.Fatalf("revoke reviewer was not reported as changed: %#v", revoked)
	}

	entityID, entityBatchID, entityProposalID := createEntityThroughProposalE2E(
		t, editor, admin, baseURL, runID+"-primary",
	)
	secondEntityID, _, _ := createEntityThroughProposalE2E(
		t, editor, admin, baseURL, runID+"-duplicate",
	)

	requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/entities?page_size=20", nil, http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/entities/%s", baseURL, entityID), nil, http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/entities/%s/mentions?page_size=10",
			baseURL, entityID), nil, http.StatusOK)
	requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/entities/%s/labels", baseURL, entityID),
		map[string]any{
			"language": "en", "label": "API E2E Entity " + runID,
			"description": "governance workflow", "is_primary": false,
		}, http.StatusCreated)
	requestE2ENoBody(t, admin, http.MethodPut,
		fmt.Sprintf("%s/api/v1/entities/%s/labels/primary", baseURL, entityID),
		http.StatusNoContent, map[string]any{
			"language": "en", "label": "API E2E Entity " + runID,
		})
	alias := requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/entities/%s/aliases", baseURL, entityID),
		map[string]any{
			"language": "en", "alias": "E2E Alias " + runID,
			"alias_type": "common",
		}, http.StatusCreated)
	aliasID := e2eUUID(t, alias, "id")
	requestE2ENoBody(t, admin, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/entities/%s/aliases/%s",
			baseURL, entityID, aliasID), http.StatusNoContent)
	requestE2ENoBody(t, admin, http.MethodPut,
		fmt.Sprintf("%s/api/v1/entities/%s/labels/primary", baseURL, entityID),
		http.StatusNoContent, map[string]any{
			"language": "und", "label": "API E2E " + runID + "-primary",
		})
	requestE2ENoBody(t, admin, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/entities/%s/labels?language=en&label=%s",
			baseURL, entityID, url.QueryEscape("API E2E Entity "+runID)),
		http.StatusNoContent)

	page := requestE2EMap(t, admin, http.MethodPost, baseURL+"/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Entity Page " + runID},
		http.StatusCreated)
	pageID := e2eUUID(t, page, "id")
	requestE2EMap(t, admin, http.MethodPut,
		fmt.Sprintf("%s/api/v1/pages/%s/entity-bindings", baseURL, pageID),
		map[string]any{
			"entity_id": entityID, "role": "primary", "language": "en",
		}, http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s/entity-bindings", baseURL, pageID),
		nil, http.StatusOK)
	requestE2ENoBody(t, admin, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/pages/%s/entity-bindings/%s?role=primary",
			baseURL, pageID, entityID), http.StatusNoContent)

	claimID, claimBatchID := createClaimThroughProposalE2E(
		t, editor, admin, baseURL, entityID,
	)
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/claims/%s", baseURL, claimID), nil, http.StatusOK)
	requestE2ENoBody(t, admin, http.MethodPut,
		fmt.Sprintf("%s/api/v1/claims/%s/verification", baseURL, claimID),
		http.StatusNoContent, map[string]any{"verification_status": "human_verified"})
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/claims/%s/usages?page_size=10", baseURL, claimID),
		nil, http.StatusOK)

	remote := requestE2EMap(t, admin, http.MethodPost,
		baseURL+"/api/v1/federated-wikis", map[string]any{
			"wiki_key":            "api-e2e-" + runID,
			"display_name":        "API E2E Remote",
			"base_url":            "https://example.com",
			"entity_url_template": "https://example.com/entities/{entity_id}",
			"trust_level":         "trusted", "status": "active",
			"metadata": map[string]any{"e2e": true},
		}, http.StatusCreated)
	remoteID := e2eUUID(t, remote, "id")
	requestE2EMap(t, admin, http.MethodPut,
		fmt.Sprintf("%s/api/v1/federated-wikis/%s", baseURL, remoteID),
		map[string]any{
			"display_name":        "API E2E Remote Updated",
			"base_url":            "https://example.com",
			"entity_url_template": "https://example.com/entities/{entity_id}",
			"trust_level":         "trusted", "status": "active",
			"metadata": map[string]any{"updated": true},
		}, http.StatusOK)
	link := requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/entities/%s/federation-links", baseURL, entityID),
		map[string]any{
			"remote_wiki_id": remoteID, "remote_entity_id": "remote-1",
			"remote_canonical_key": "concept:remote", "remote_label": "Remote",
			"relation_type": "same_as", "verification_status": "human_verified",
			"metadata": map[string]any{},
		}, http.StatusCreated)
	linkID := e2eUUID(t, link, "id")
	requestE2EMap(t, admin, http.MethodPut,
		fmt.Sprintf("%s/api/v1/federation-links/%s", baseURL, linkID),
		map[string]any{
			"remote_canonical_key": "concept:remote-updated",
			"remote_label":         "Remote Updated", "relation_type": "same_as",
			"verification_status": "human_verified", "status": "active",
			"metadata": map[string]any{"updated": true},
		}, http.StatusOK)
	for _, endpoint := range []string{
		"/api/v1/federated-wikis?include_disabled=true",
		"/api/v1/federation-links?page_size=10",
		fmt.Sprintf("/api/v1/entities/%s/federation-links", entityID),
		fmt.Sprintf("/api/v1/entities/%s/graph", entityID),
	} {
		requestE2EMap(t, admin, http.MethodGet, baseURL+endpoint, nil, http.StatusOK)
	}
	requestE2EMap(t, admin, http.MethodPost,
		baseURL+"/api/v1/entity-graph/rebuild", map[string]any{}, http.StatusOK)

	merge := requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/entities/%s/merge", baseURL, secondEntityID),
		map[string]any{
			"target_entity_id": entityID, "reason": "API E2E duplicate",
		}, http.StatusOK)
	mergeID := e2eUUID(t, merge, "id")
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/entities/%s/merge", baseURL, secondEntityID),
		nil, http.StatusOK)
	requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/entity-merges/%s/rollback", baseURL, mergeID),
		map[string]any{}, http.StatusOK)

	requestE2EMap(t, admin, http.MethodPost,
		baseURL+"/api/v1/fact-consistency-scans", map[string]any{}, http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/fact-consistency-issues?page_size=20", nil, http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/roles", nil, http.StatusOK)

	protection := requestE2EMap(t, admin, http.MethodPost,
		baseURL+"/api/v1/page-protections", map[string]any{
			"page_id": pageID, "action_type": "edit", "required_role_key": "editor",
		}, http.StatusCreated)
	protectionID := e2eUUID(t, protection, "id")
	requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/page-protections?include_expired=true", nil, http.StatusOK)
	requestE2ENoBody(t, admin, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/page-protections/%s", baseURL, protectionID),
		http.StatusNoContent)

	tag := requestE2EMap(t, admin, http.MethodPost,
		baseURL+"/api/v1/change-tags", map[string]any{
			"tag_key": "api-e2e-" + runID, "name": "API E2E",
			"description": "isolated workflow",
		}, http.StatusCreated)
	tagID := e2eUUID(t, tag, "id")
	requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/change-tags/%s/assignments", baseURL, tagID),
		map[string]any{"target_type": "proposal", "target_id": entityProposalID},
		http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/change-tags", nil, http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/audit-events?page_size=20&change_batch_id=%s",
			baseURL, entityBatchID), nil, http.StatusOK)

	requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/change-batches/%s/rollback", baseURL, claimBatchID),
		map[string]any{}, http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/apply-queue?page_size=20", nil, http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/proposals?page_size=20", nil, http.StatusOK)

	profiles := requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/ai-trust-profiles", nil, http.StatusOK)
	if items, ok := profiles["items"].([]any); ok && len(items) > 0 {
		item, _ := items[0].(map[string]any)
		actorID := e2eUUID(t, item, "actor_id")
		requestE2EMap(t, admin, http.MethodPut,
			fmt.Sprintf("%s/api/v1/ai-trust-profiles/%s", baseURL, actorID),
			map[string]any{
				"trust_level": "assisted", "required_sample_percent": 100,
			}, http.StatusOK)
	}

	requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/revision-storage", nil, http.StatusOK)
	requestE2EMap(t, admin, http.MethodPost,
		baseURL+"/api/v1/revision-storage/archive",
		map[string]any{"limit": 10}, http.StatusOK)
	_ = adminActor
}

func TestAPIBulkReviewE2E(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("API_E2E_BASE_URL"), "/")
	password := os.Getenv("API_E2E_PASSWORD")
	if baseURL == "" || password == "" {
		t.Skip("set API_E2E_BASE_URL and API_E2E_PASSWORD")
	}
	runID := strconv.FormatInt(time.Now().UnixNano(), 36)
	admin, _ := apiE2EAdmin(t, baseURL, password)
	editor, _ := registerE2EUser(
		t, baseURL, "bulk_editor_"+runID,
		"bulk_editor_"+runID+"@example.invalid", password,
	)
	proposalID, _ := createEntityProposalE2E(
		t, editor, baseURL, "bulk-"+runID,
	)
	requestE2EMap(t, editor, http.MethodPost,
		fmt.Sprintf("%s/api/v1/proposals/%s/submit", baseURL, proposalID),
		map[string]any{}, http.StatusOK)

	batch := requestE2EMap(t, admin, http.MethodPost,
		baseURL+"/api/v1/bulk-review-batches", map[string]any{
			"proposal_ids": []any{proposalID}, "sample_percent": 100,
			"force_full": false, "wave_size": 1,
		}, http.StatusCreated)
	batchID := e2eUUID(t, batch, "id")
	requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/bulk-review-batches?page_size=20",
		nil, http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/bulk-review-batches/%s", baseURL, batchID),
		nil, http.StatusOK)
	requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/bulk-review-batches/%s/proposals/%s/decision",
			baseURL, batchID, proposalID),
		map[string]any{"approve": true, "reason": "API E2E bulk approval"},
		http.StatusOK)
	finalized := requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/bulk-review-batches/%s/finalize", baseURL, batchID),
		map[string]any{}, http.StatusOK)
	if finalized["status"] != "ready" {
		t.Fatalf("bulk review status after finalize=%v, want ready", finalized["status"])
	}
	paused := requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/bulk-review-batches/%s/pause", baseURL, batchID),
		map[string]any{}, http.StatusOK)
	if paused["status"] != "paused" {
		t.Fatalf("bulk review status after pause=%v, want paused", paused["status"])
	}
	resumed := requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/bulk-review-batches/%s/resume", baseURL, batchID),
		map[string]any{}, http.StatusOK)
	if resumed["status"] != "ready" {
		t.Fatalf("bulk review status after resume=%v, want ready", resumed["status"])
	}
	wave := requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/bulk-review-batches/%s/apply-next-wave",
			baseURL, batchID), map[string]any{}, http.StatusOK)
	if wave["status"] != "completed" {
		t.Fatalf("bulk review status after apply=%v, want completed", wave["status"])
	}
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/bulk-review-batches/%s/audit-events",
			baseURL, batchID), nil, http.StatusOK)
}

func TestAPIImportAndAIConfigE2E(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("API_E2E_BASE_URL"), "/")
	password := os.Getenv("API_E2E_PASSWORD")
	if baseURL == "" || password == "" {
		t.Skip("set API_E2E_BASE_URL and API_E2E_PASSWORD")
	}
	runID := strconv.FormatInt(time.Now().UnixNano(), 36)
	admin, _ := apiE2EAdmin(t, baseURL, password)

	requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/admin/ai-config", nil, http.StatusOK)
	configBody := map[string]any{
		"enabled": true, "provider": "openai-compatible",
		"base_url": "https://api-e2e.invalid/v1", "model": "api-e2e-model",
		"response_format": "json_schema", "max_input_tokens": 4096,
		"chunk_characters": 1000, "request_timeout_seconds": 5,
		"max_attempts": 1, "api_key": "api-e2e-not-a-real-key",
	}
	config := requestE2EMap(t, admin, http.MethodPut,
		baseURL+"/api/v1/admin/ai-config", configBody, http.StatusOK)
	if config["api_key_configured"] != true {
		t.Fatalf("AI config did not report an encrypted credential: %#v", config)
	}
	if _, exposed := config["api_key"]; exposed {
		t.Fatal("AI config response exposed api_key")
	}
	delete(configBody, "api_key")
	configBody["api_key"] = ""
	config = requestE2EMap(t, admin, http.MethodPut,
		baseURL+"/api/v1/admin/ai-config", configBody, http.StatusOK)
	if config["api_key_configured"] != true {
		t.Fatal("blank AI config update did not preserve the credential")
	}
	response := doE2ERequest(t, admin, http.MethodPost,
		baseURL+"/api/v1/admin/ai-config/test", map[string]any{}, nil)
	testBody := readE2EBody(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusBadGateway &&
		response.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("AI config test status=%d want 502 or 504: %s",
			response.StatusCode, testBody)
	}
	configBody["enabled"] = false
	requestE2EMap(t, admin, http.MethodPut,
		baseURL+"/api/v1/admin/ai-config", configBody, http.StatusOK)

	cancelled := requestE2EMapHeaders(t, admin, http.MethodPost,
		baseURL+"/api/v1/import-jobs", map[string]any{
			"job_type": "source_import",
			"config": map[string]any{
				"source": map[string]any{
					"kind": "url", "url": "https://example.com/api-e2e-" + runID,
				},
				"title": "API E2E Cancel " + runID,
			},
		}, http.Header{"Idempotency-Key": []string{uuid.NewString()}},
		http.StatusCreated)
	cancelledID := e2eUUID(t, cancelled, "id")
	requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/import-jobs/%s/cancel", baseURL, cancelledID),
		map[string]any{}, http.StatusOK)
	requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/import-jobs/%s/retry", baseURL, cancelledID),
		map[string]any{}, http.StatusOK)
	requestE2EMap(t, admin, http.MethodPost,
		fmt.Sprintf("%s/api/v1/import-jobs/%s/cancel", baseURL, cancelledID),
		map[string]any{}, http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/import-jobs/%s", baseURL, cancelledID),
		nil, http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/import-jobs?status=cancelled&page_size=20",
		nil, http.StatusOK)

	configBody["enabled"] = true
	requestE2EMap(t, admin, http.MethodPut,
		baseURL+"/api/v1/admin/ai-config", configBody, http.StatusOK)
	title := "API E2E Upload " + runID
	upload := createImportUploadE2E(t, admin, baseURL, title,
		[]byte("Anby Wiki API E2E evidence text.\nThis source validates immutable chunks.\n"))
	uploadID := e2eUUID(t, upload, "id")
	detail := waitForE2EImportTerminal(t, admin,
		fmt.Sprintf("%s/api/v1/import-jobs/%s", baseURL, uploadID))
	job, ok := detail["job"].(map[string]any)
	if !ok {
		t.Fatalf("import detail has no job: %#v", detail)
	}
	sourceVersionID := e2eUUID(t, job, "source_version_id")
	chunks := requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/source-versions/%s/chunks?page_size=20",
			baseURL, sourceVersionID), nil, http.StatusOK)
	items, ok := chunks["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("parsed import exposed no source chunks: %#v", chunks)
	}
	citation := requestE2EMap(t, admin, http.MethodPost,
		baseURL+"/api/v1/citations",
		map[string]any{"source_version_id": sourceVersionID},
		http.StatusCreated)
	citationID := e2eUUID(t, citation, "id")
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/citations/%s", baseURL, citationID),
		nil, http.StatusOK)
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/citations/%s/usages?page_size=20",
			baseURL, citationID), nil, http.StatusOK)

	sources := requestE2EMap(t, admin, http.MethodGet,
		baseURL+"/api/v1/sources?q="+url.QueryEscape(title)+"&page_size=20",
		nil, http.StatusOK)
	sourceItems, ok := sources["items"].([]any)
	if !ok || len(sourceItems) == 0 {
		t.Fatalf("import source was not discoverable: %#v", sources)
	}
	source, ok := sourceItems[0].(map[string]any)
	if !ok {
		t.Fatalf("invalid source item: %#v", sourceItems[0])
	}
	sourceID := e2eUUID(t, source, "id")
	requestE2EMap(t, admin, http.MethodGet,
		fmt.Sprintf("%s/api/v1/sources/%s/versions?page_size=20",
			baseURL, sourceID), nil, http.StatusOK)
}

func testPageWorkflow(
	t *testing.T,
	client *http.Client,
	baseURL, runID string,
) {
	t.Helper()
	first := requestE2EMap(t, client, http.MethodPost, baseURL+"/api/v1/pages",
		map[string]any{"namespace": "main", "title": "API E2E A " + runID,
			"language": "en", "content_model": "block-v1"}, http.StatusCreated)
	second := requestE2EMap(t, client, http.MethodPost, baseURL+"/api/v1/pages",
		map[string]any{"namespace": "main", "title": "API E2E B " + runID,
			"language": "en", "content_model": "block-v1"}, http.StatusCreated)
	firstID := e2eUUID(t, first, "id")
	secondID := e2eUUID(t, second, "id")
	firstHeading := uuid.New()
	secondHeading := uuid.New()

	firstRevision := requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/pages/%s/revisions", baseURL, firstID),
		map[string]any{
			"ast":     documentE2E(firstHeading, "API E2E Section", "first revision"),
			"summary": "api e2e first",
		}, http.StatusCreated)
	secondRevision := requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/pages/%s/revisions", baseURL, secondID),
		map[string]any{
			"ast":     documentE2E(secondHeading, "Target Section", "target"),
			"summary": "api e2e target",
		}, http.StatusCreated)
	firstRevisionID := e2eUUID(t, firstRevision, "id")
	secondRevisionID := e2eUUID(t, secondRevision, "id")

	updatedRevision := requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/pages/%s/revisions", baseURL, firstID),
		map[string]any{
			"expected_revision_id": firstRevisionID,
			"ast":                  documentE2E(firstHeading, "API E2E Section", "second revision"),
			"summary":              "api e2e second",
		}, http.StatusCreated)
	updatedRevisionID := e2eUUID(t, updatedRevision, "id")

	requestE2EMap(t, client, http.MethodGet,
		baseURL+"/api/v1/pages?page_size=10", nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s", baseURL, firstID), nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		baseURL+"/api/v1/pages/by-title?namespace=main&title="+
			url.QueryEscape("API E2E A "+runID), nil, http.StatusOK)
	renamedTitle := "API E2E A Renamed " + runID
	requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/pages/%s/rename", baseURL, firstID),
		map[string]any{"title": renamedTitle}, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		baseURL+"/api/v1/pages/by-title?namespace=main&title="+
			url.QueryEscape(renamedTitle), nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		baseURL+"/api/v1/pages/search?q="+url.QueryEscape(renamedTitle),
		nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		baseURL+"/api/v1/search/capabilities", nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s/revisions?page_size=10", baseURL, firstID),
		nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s/revisions/%s",
			baseURL, firstID, updatedRevisionID), nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s/diff?from=%s&to=%s",
			baseURL, firstID, firstRevisionID, updatedRevisionID), nil, http.StatusOK)
	testPageProposalPreviewE2E(
		t, client, baseURL, firstID, updatedRevisionID,
	)
	requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/pages/%s/rollback", baseURL, firstID),
		map[string]any{"target_revision_id": firstRevisionID, "summary": "api e2e rollback"},
		http.StatusCreated)

	requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/pages/%s/redirect", baseURL, secondID),
		map[string]any{"target": map[string]any{
			"kind": "page", "target_page_id": firstID,
		}}, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s/redirect", baseURL, secondID),
		nil, http.StatusOK)
	requestE2ENoBody(t, client, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/pages/%s/redirect", baseURL, secondID),
		http.StatusNoContent)

	slug := waitForE2EOutline(t, client,
		fmt.Sprintf("%s/api/v1/pages/%s/outline", baseURL, firstID),
		firstHeading)
	sectionKey := waitForE2ESections(t, client,
		fmt.Sprintf("%s/api/v1/pages/%s/sections", baseURL, firstID),
		firstHeading)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s/sections/%s",
			baseURL, firstID, url.PathEscape(sectionKey)), nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s/section-locator/%s",
			baseURL, firstID, firstHeading), nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s/anchors/%s",
			baseURL, firstID, url.PathEscape(slug)), nil, http.StatusOK)
	legacyBlockID := uuid.New()
	requestE2EMap(t, client, http.MethodPut,
		fmt.Sprintf("%s/api/v1/pages/%s/block-redirects/%s",
			baseURL, firstID, legacyBlockID),
		map[string]any{
			"target_page_id": firstID, "target_block_id": firstHeading,
		}, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s/block-redirects", baseURL, firstID),
		nil, http.StatusOK)
	requestE2ENoBody(t, client, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/pages/%s/block-redirects/%s",
			baseURL, firstID, legacyBlockID), http.StatusNoContent)

	_ = waitForE2EOutline(t, client,
		fmt.Sprintf("%s/api/v1/pages/%s/outline", baseURL, firstID),
		firstHeading)
	for _, endpoint := range []string{
		fmt.Sprintf("/api/v1/pages/%s/backlinks?page_size=10", firstID),
		fmt.Sprintf("/api/v1/pages/%s/references", firstID),
		fmt.Sprintf("/api/v1/pages/%s/related", firstID),
		fmt.Sprintf("/api/v1/pages/%s/sections", firstID),
		fmt.Sprintf("/api/v1/pages/%s/collections", firstID),
	} {
		requestE2EMap(t, client, http.MethodGet, baseURL+endpoint, nil, http.StatusOK)
	}

	_ = secondRevisionID
}

func testAssetWorkflow(t *testing.T, client *http.Client, baseURL, runID string) {
	t.Helper()
	png, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
	)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "api-e2e.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, writeErr := part.Write(png); writeErr != nil {
		t.Fatal(writeErr)
	}
	if writeErr := writer.WriteField("name", "API E2E Asset "+runID); writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/assets", &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("upload asset status=%d: %s",
			response.StatusCode, readE2EBody(response.Body))
	}
	var asset map[string]any
	if err := json.NewDecoder(response.Body).Decode(&asset); err != nil {
		t.Fatal(err)
	}
	revision, ok := asset["current_revision"].(map[string]any)
	if !ok {
		t.Fatalf("asset has no current_revision: %#v", asset)
	}
	revisionID := e2eUUID(t, revision, "id")
	requestE2EMap(t, client, http.MethodGet,
		baseURL+"/api/v1/assets?page_size=10", nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/assets/revisions/%s", baseURL, revisionID),
		nil, http.StatusOK)
	response = doE2ERequest(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/assets/revisions/%s/content", baseURL, revisionID),
		nil, nil)
	assertE2EStatus(t, response, http.StatusOK)
}

func createImportUploadE2E(
	t *testing.T,
	client *http.Client,
	baseURL, title string,
	content []byte,
) map[string]any {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "api-e2e.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"title": title, "instructions": "API E2E evidence import",
		"route_mode": "auto",
	} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(
		http.MethodPost, baseURL+"/api/v1/import-jobs/uploads", &body,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Idempotency-Key", uuid.NewString())
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("upload import status=%d: %s",
			response.StatusCode, readE2EBody(response.Body))
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func waitForE2EImportTerminal(
	t *testing.T,
	client *http.Client,
	endpoint string,
) map[string]any {
	t.Helper()
	var lastStatus int
	var lastBody string
	for range 120 {
		response := doE2ERequest(t, client, http.MethodGet, endpoint, nil, nil)
		lastStatus = response.StatusCode
		lastBody = readE2EBody(response.Body)
		response.Body.Close()
		if lastStatus == http.StatusOK {
			var detail map[string]any
			if json.Unmarshal([]byte(lastBody), &detail) == nil {
				if job, ok := detail["job"].(map[string]any); ok {
					switch job["status"] {
					case "succeeded", "failed", "cancelled":
						return detail
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("GET %s did not reach a terminal import state; status=%d: %s",
		endpoint, lastStatus, lastBody)
	return nil
}

func testSourceWorkflow(t *testing.T, client *http.Client, baseURL, runID string) {
	t.Helper()
	source := requestE2EMap(t, client, http.MethodPost,
		baseURL+"/api/v1/sources", map[string]any{
			"source_type": "webpage",
			"url":         "https://example.com/api-e2e-" + runID,
			"title":       "API E2E Source " + runID,
			"metadata":    map[string]any{"e2e": true},
		}, http.StatusCreated)
	sourceID := e2eUUID(t, source, "id")
	requestE2EMap(t, client, http.MethodGet,
		baseURL+"/api/v1/sources?page_size=10", nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/sources/%s", baseURL, sourceID), nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/sources/%s/versions?page_size=10", baseURL, sourceID),
		nil, http.StatusOK)
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/sources/%s/usages?page_size=10", baseURL, sourceID),
		nil, http.StatusOK)
	page := requestE2EMap(t, client, http.MethodPost, baseURL+"/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Source Usage " + runID},
		http.StatusCreated)
	pageID := e2eUUID(t, page, "id")
	requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/sources/%s/usages/%s?page_size=10",
			baseURL, sourceID, pageID), nil, http.StatusOK)
}

func testDatasetWorkflow(t *testing.T, client *http.Client, baseURL, runID string) {
	t.Helper()
	dataset := requestE2EMap(t, client, http.MethodPost,
		baseURL+"/api/v1/datasets", map[string]any{
			"name": "API E2E Dataset " + runID,
			"schema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"name": map[string]any{"type": "string"}},
				"required":   []string{"name"}, "additionalProperties": false,
			},
		}, http.StatusCreated)
	datasetID := e2eUUID(t, dataset, "id")
	record := requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/datasets/%s/records", baseURL, datasetID),
		map[string]any{"values": map[string]any{"name": "first"}}, http.StatusCreated)
	recordID := e2eUUID(t, record, "id")
	requestE2EMap(t, client, http.MethodPut,
		fmt.Sprintf("%s/api/v1/dataset-records/%s", baseURL, recordID),
		map[string]any{"values": map[string]any{"name": "updated"}}, http.StatusOK)
	view := requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/datasets/%s/views", baseURL, datasetID),
		map[string]any{
			"name": "API E2E View", "view_type": "table",
			"config": map[string]any{"columns": []string{"name"}},
		}, http.StatusCreated)
	viewID := e2eUUID(t, view, "id")
	for _, endpoint := range []string{
		"/api/v1/datasets?page_size=10",
		fmt.Sprintf("/api/v1/datasets/%s", datasetID),
		fmt.Sprintf("/api/v1/datasets/%s/records?page_size=10", datasetID),
		fmt.Sprintf("/api/v1/datasets/%s/views", datasetID),
		fmt.Sprintf("/api/v1/dataset-views/%s", viewID),
		fmt.Sprintf("/api/v1/dataset-views/%s/records?page_size=10", viewID),
	} {
		requestE2EMap(t, client, http.MethodGet, baseURL+endpoint, nil, http.StatusOK)
	}
}

func testComponentWorkflow(t *testing.T, client *http.Client, baseURL, runID string) {
	t.Helper()
	component := requestE2EMap(t, client, http.MethodPost,
		baseURL+"/api/v1/components", map[string]any{
			"component_key": "api-e2e-" + runID,
			"name":          "API E2E Component " + runID,
		}, http.StatusCreated)
	componentID := e2eUUID(t, component, "id")
	versionBody := map[string]any{
		"props_schema": map[string]any{
			"type": "object", "additionalProperties": true,
		},
		"renderer_ref": "builtin.key_value",
	}
	version := requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/components/%s/versions", baseURL, componentID),
		versionBody, http.StatusCreated)
	versionNumber := int(version["version"].(float64))
	requestE2EMap(t, client, http.MethodPut,
		fmt.Sprintf("%s/api/v1/components/%s/versions/%d",
			baseURL, componentID, versionNumber), versionBody, http.StatusOK)
	requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/components/%s/versions/%d/preview",
			baseURL, componentID, versionNumber),
		map[string]any{"props": map[string]any{"name": "value"}}, http.StatusOK)
	requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/components/%s/versions/%d/publish",
			baseURL, componentID, versionNumber), map[string]any{}, http.StatusOK)
	requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/components/%s/versions/%d/deprecate",
			baseURL, componentID, versionNumber), map[string]any{}, http.StatusOK)
	for _, endpoint := range []string{
		"/api/v1/components?page_size=10",
		fmt.Sprintf("/api/v1/components/%s", componentID),
		fmt.Sprintf("/api/v1/components/%s/versions", componentID),
		fmt.Sprintf("/api/v1/components/%s/usages?page_size=10", componentID),
	} {
		requestE2EMap(t, client, http.MethodGet, baseURL+endpoint, nil, http.StatusOK)
	}
}

func testCollectionWorkflow(t *testing.T, client *http.Client, baseURL, runID string) {
	t.Helper()
	page := requestE2EMap(t, client, http.MethodPost, baseURL+"/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Collection Page " + runID},
		http.StatusCreated)
	pageID := e2eUUID(t, page, "id")
	headingID := uuid.New()
	revision := requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/pages/%s/revisions", baseURL, pageID),
		map[string]any{"ast": documentE2E(headingID, "Collection", "member")},
		http.StatusCreated)
	revisionID := e2eUUID(t, revision, "id")
	collection := requestE2EMap(t, client, http.MethodPost,
		baseURL+"/api/v1/collections", map[string]any{
			"collection_type": "manual", "title": "API E2E Collection " + runID,
		}, http.StatusCreated)
	collectionID := e2eUUID(t, collection, "id")
	requestE2ENoBody(t, client, http.MethodPut,
		fmt.Sprintf("%s/api/v1/collections/%s/members", baseURL, collectionID),
		http.StatusNoContent, map[string]any{"items": []any{map[string]any{
			"member_type": "page", "page_id": pageID,
			"sort_key": "0001", "source_revision_id": revisionID,
		}}})
	for _, endpoint := range []string{
		"/api/v1/collections?page_size=10",
		fmt.Sprintf("/api/v1/collections/%s", collectionID),
		fmt.Sprintf("/api/v1/collections/%s/members?page_size=10", collectionID),
		fmt.Sprintf("/api/v1/pages/%s/collections", pageID),
	} {
		requestE2EMap(t, client, http.MethodGet, baseURL+endpoint, nil, http.StatusOK)
	}
	rule := requestE2EMap(t, client, http.MethodPost,
		baseURL+"/api/v1/collections", map[string]any{
			"collection_type": "rule", "title": "API E2E Rule " + runID,
			"query": map[string]any{
				"version": 1, "kind": "entity_type", "entity_type": "concept",
			},
		}, http.StatusCreated)
	ruleID := e2eUUID(t, rule, "id")
	requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/collections/%s/rebuild", baseURL, ruleID),
		map[string]any{"source_revision_id": revisionID}, http.StatusOK)
}

func createEntityThroughProposalE2E(
	t *testing.T,
	proposer, reviewer *http.Client,
	baseURL, suffix string,
) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	proposalID, entityID := createEntityProposalE2E(
		t, proposer, baseURL, suffix,
	)
	result := approveAndApplyProposalE2E(
		t, proposer, reviewer, baseURL, proposalID,
	)
	entityIDs := e2eUUIDSlice(t, result, "entity_ids")
	if len(entityIDs) != 1 || entityIDs[0] != entityID {
		t.Fatalf("apply entity ids=%v want=%s", entityIDs, entityID)
	}
	return entityID, e2eUUID(t, result, "change_batch_id"), proposalID
}

func testPageProposalPreviewE2E(
	t *testing.T,
	client *http.Client,
	baseURL string,
	pageID, baseRevisionID uuid.UUID,
) {
	t.Helper()
	proposal := requestE2EMapHeaders(
		t, client, http.MethodPost, baseURL+"/api/v1/proposals",
		map[string]any{
			"target_type": "page", "target_id": pageID,
			"base_revision_id": baseRevisionID, "risk_level": "medium",
		},
		http.Header{"Idempotency-Key": []string{uuid.NewString()}},
		http.StatusCreated,
	)
	proposalID := e2eUUID(t, proposal, "id")
	requestE2EMap(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/proposals/%s/operations", baseURL, proposalID),
		map[string]any{
			"schema_version": 1, "operation_type": "insert_block",
			"base":          map[string]any{"revision_id": baseRevisionID},
			"target":        map[string]any{"page_id": pageID},
			"expected_hash": nil,
			"evidence": []any{
				map[string]any{"note": "API E2E read-only preview"},
			},
			"risk": map[string]any{
				"level": "medium", "reasons": []string{"api e2e preview"},
			},
			"payload": map[string]any{
				"index": 2,
				"block": map[string]any{
					"id": uuid.New(), "type": "paragraph",
					"content": []any{
						map[string]any{"type": "text", "text": "preview only"},
					},
				},
			},
		}, http.StatusCreated)
	preview := requestE2EMap(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/proposals/%s/preview", baseURL, proposalID),
		nil, http.StatusOK)
	impact, ok := preview["impact"].(map[string]any)
	if !ok || impact["added_blocks"] != float64(1) {
		t.Fatalf("proposal preview impact=%#v, want one added block", preview["impact"])
	}
}

func createEntityProposalE2E(
	t *testing.T,
	proposer *http.Client,
	baseURL, suffix string,
) (uuid.UUID, uuid.UUID) {
	t.Helper()
	entityID := uuid.New()
	proposal := requestE2EMapHeaders(
		t, proposer, http.MethodPost, baseURL+"/api/v1/proposals",
		map[string]any{
			"target_type": "entity", "risk_level": "medium",
			"base_state_version": 0,
		},
		http.Header{"Idempotency-Key": []string{uuid.NewString()}},
		http.StatusCreated,
	)
	proposalID := e2eUUID(t, proposal, "id")
	requestE2EMap(t, proposer, http.MethodPost,
		fmt.Sprintf("%s/api/v1/proposals/%s/operations", baseURL, proposalID),
		map[string]any{
			"schema_version": 1, "operation_type": "create_entity",
			"base": map[string]any{"state_version": 0},
			"target": map[string]any{
				"wiki_id":   "00000000-0000-7000-8000-000000000001",
				"entity_id": entityID,
			},
			"expected_hash": nil,
			"evidence":      []any{map[string]any{"note": "API E2E isolated entity"}},
			"risk": map[string]any{
				"level": "medium", "reasons": []string{"api e2e review"},
			},
			"payload": map[string]any{
				"type_key": "concept", "canonical_key": "api-e2e-" + suffix,
				"labels": []any{map[string]any{
					"language": "und", "label": "API E2E " + suffix,
					"description": "", "is_primary": true,
				}},
			},
		}, http.StatusCreated)
	return proposalID, entityID
}

func createClaimThroughProposalE2E(
	t *testing.T,
	proposer, reviewer *http.Client,
	baseURL string,
	entityID uuid.UUID,
) (uuid.UUID, uuid.UUID) {
	t.Helper()
	proposal := requestE2EMapHeaders(
		t, proposer, http.MethodPost, baseURL+"/api/v1/proposals",
		map[string]any{
			"target_type": "entity", "target_id": entityID,
			"risk_level": "medium", "base_state_version": 0,
		},
		http.Header{"Idempotency-Key": []string{uuid.NewString()}},
		http.StatusCreated,
	)
	proposalID := e2eUUID(t, proposal, "id")
	requestE2EMap(t, proposer, http.MethodPost,
		fmt.Sprintf("%s/api/v1/proposals/%s/operations", baseURL, proposalID),
		map[string]any{
			"schema_version": 1, "operation_type": "create_claim",
			"base":          map[string]any{"state_version": 0},
			"target":        map[string]any{"entity_id": entityID},
			"expected_hash": nil,
			"evidence":      []any{map[string]any{"note": "API E2E isolated claim"}},
			"risk": map[string]any{
				"level": "medium", "reasons": []string{"api e2e review"},
			},
			"payload": map[string]any{
				"property_key": "release_date",
				"value":        map[string]any{"date": "2026-08-18"},
				"qualifiers":   map[string]any{}, "rank": "normal",
				"origin_type": "human", "citation_ids": []any{},
			},
		}, http.StatusCreated)
	result := approveAndApplyProposalE2E(
		t, proposer, reviewer, baseURL, proposalID,
	)
	claimIDs := e2eUUIDSlice(t, result, "claim_ids")
	if len(claimIDs) != 1 {
		t.Fatalf("apply claim ids=%v, want one", claimIDs)
	}
	return claimIDs[0], e2eUUID(t, result, "change_batch_id")
}

func approveAndApplyProposalE2E(
	t *testing.T,
	proposer, reviewer *http.Client,
	baseURL string,
	proposalID uuid.UUID,
) map[string]any {
	t.Helper()
	requestE2EMap(t, proposer, http.MethodGet,
		fmt.Sprintf("%s/api/v1/proposals/%s", baseURL, proposalID),
		nil, http.StatusOK)
	submitted := requestE2EMap(t, proposer, http.MethodPost,
		fmt.Sprintf("%s/api/v1/proposals/%s/submit", baseURL, proposalID),
		map[string]any{}, http.StatusOK)
	if rawTask, ok := submitted["review_task"]; ok && rawTask != nil {
		task, ok := rawTask.(map[string]any)
		if !ok {
			t.Fatalf("invalid review_task: %#v", rawTask)
		}
		taskID := e2eUUID(t, task, "id")
		requestE2EMap(t, reviewer, http.MethodGet,
			baseURL+"/api/v1/review-tasks?page_size=100", nil, http.StatusOK)
		requestE2EMap(t, reviewer, http.MethodPost,
			fmt.Sprintf("%s/api/v1/review-tasks/%s/decision", baseURL, taskID),
			map[string]any{"approve": true, "reason": "API E2E approval"},
			http.StatusOK)
	}
	requestE2EMap(t, reviewer, http.MethodGet,
		baseURL+"/api/v1/apply-queue?page_size=100", nil, http.StatusOK)
	return requestE2EMap(t, reviewer, http.MethodPost,
		fmt.Sprintf("%s/api/v1/proposals/%s/apply", baseURL, proposalID),
		map[string]any{}, http.StatusOK)
}

func documentE2E(headingID uuid.UUID, heading, paragraph string) map[string]any {
	return map[string]any{
		"type": "document", "schema_version": 1,
		"children": []any{
			map[string]any{
				"id": headingID, "type": "heading", "level": 2,
				"content": []any{map[string]any{"type": "text", "text": heading}},
			},
			map[string]any{
				"id": uuid.New(), "type": "paragraph",
				"content": []any{map[string]any{"type": "text", "text": paragraph}},
			},
		},
	}
}

func requestE2EMapHeaders(
	t *testing.T,
	client *http.Client,
	method, endpoint string,
	body any,
	headers http.Header,
	wantStatus int,
) map[string]any {
	t.Helper()
	response := doE2ERequest(t, client, method, endpoint, body, headers)
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s status=%d want=%d: %s",
			method, endpoint, response.StatusCode, wantStatus,
			readE2EBody(response.Body))
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode %s %s: %v", method, endpoint, err)
	}
	return result
}

func requestE2EMap(
	t *testing.T,
	client *http.Client,
	method, endpoint string,
	body any,
	wantStatus int,
) map[string]any {
	t.Helper()
	response := doE2ERequest(t, client, method, endpoint, body, nil)
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s status=%d want=%d: %s",
			method, endpoint, response.StatusCode, wantStatus,
			readE2EBody(response.Body))
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode %s %s: %v", method, endpoint, err)
	}
	return result
}

func requestE2ENoBody(
	t *testing.T,
	client *http.Client,
	method, endpoint string,
	wantStatus int,
	body ...any,
) {
	t.Helper()
	var payload any
	if len(body) > 0 {
		payload = body[0]
	}
	response := doE2ERequest(t, client, method, endpoint, payload, nil)
	assertE2EStatus(t, response, wantStatus)
}

func e2eUUID(t *testing.T, value map[string]any, key string) uuid.UUID {
	t.Helper()
	raw, ok := value[key].(string)
	if !ok {
		t.Fatalf("%s is not a string UUID in %#v", key, value)
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", key, err)
	}
	return parsed
}

func e2eUUIDSlice(t *testing.T, value map[string]any, key string) []uuid.UUID {
	t.Helper()
	raw, ok := value[key].([]any)
	if !ok {
		t.Fatalf("%s is not an array in %#v", key, value)
	}
	result := make([]uuid.UUID, len(raw))
	for index, item := range raw {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("%s[%d] is not a UUID string", key, index)
		}
		parsed, err := uuid.Parse(text)
		if err != nil {
			t.Fatalf("parse %s[%d]: %v", key, index, err)
		}
		result[index] = parsed
	}
	return result
}

func waitForE2EOutline(
	t *testing.T,
	client *http.Client,
	endpoint string,
	headingID uuid.UUID,
) string {
	t.Helper()
	var lastStatus int
	var lastBody string
	for range 30 {
		response := doE2ERequest(t, client, http.MethodGet, endpoint, nil, nil)
		lastStatus = response.StatusCode
		lastBody = readE2EBody(response.Body)
		response.Body.Close()
		if lastStatus == http.StatusOK {
			var outline struct {
				Items []struct {
					HeadingBlockID uuid.UUID `json:"heading_block_id"`
					Slug           string    `json:"slug"`
				} `json:"items"`
			}
			if json.Unmarshal([]byte(lastBody), &outline) == nil {
				for _, item := range outline.Items {
					if item.HeadingBlockID == headingID {
						return item.Slug
					}
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("GET %s did not expose heading %s; status=%d: %s",
		endpoint, headingID, lastStatus, lastBody)
	return ""
}

func waitForE2ESections(
	t *testing.T,
	client *http.Client,
	endpoint string,
	headingID uuid.UUID,
) string {
	t.Helper()
	var lastStatus int
	var lastBody string
	for range 30 {
		response := doE2ERequest(t, client, http.MethodGet, endpoint, nil, nil)
		lastStatus = response.StatusCode
		lastBody = readE2EBody(response.Body)
		response.Body.Close()
		if lastStatus == http.StatusOK {
			var manifest struct {
				Ready bool `json:"ready"`
				Items []struct {
					Key            string    `json:"key"`
					HeadingBlockID uuid.UUID `json:"heading_block_id"`
				} `json:"items"`
			}
			if json.Unmarshal([]byte(lastBody), &manifest) == nil && manifest.Ready {
				for _, item := range manifest.Items {
					if item.HeadingBlockID == headingID {
						return item.Key
					}
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("GET %s did not expose section for heading %s; status=%d: %s",
		endpoint, headingID, lastStatus, lastBody)
	return ""
}
