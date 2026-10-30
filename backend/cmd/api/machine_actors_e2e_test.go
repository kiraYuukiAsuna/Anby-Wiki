package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/wikicli"
)

func TestAPIMachineCLIActorsE2E(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("API_E2E_BASE_URL"), "/")
	password := os.Getenv("API_E2E_PASSWORD")
	if baseURL == "" || password == "" {
		t.Skip("set API_E2E_BASE_URL and API_E2E_PASSWORD")
	}
	runID := strconv.FormatInt(time.Now().UnixNano(), 36)
	admin, adminID := apiE2EAdmin(t, baseURL, password)
	session := requestE2EMap(t, admin, http.MethodGet, baseURL+"/api/v1/auth/session", nil, http.StatusOK)
	wikiID := e2eUUID(t, session, "wiki_id")
	other, _ := registerE2EUser(t, baseURL, "machine_"+runID, "machine_"+runID+"@example.invalid", password)
	page := requestE2EMap(t, admin, http.MethodPost, baseURL+"/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Machine read " + runID}, http.StatusCreated)
	pageID := e2eUUID(t, page, "id")
	requestE2EMap(t, admin, http.MethodPost, fmt.Sprintf("%s/api/v1/pages/%s/revisions", baseURL, pageID),
		map[string]any{"ast": documentE2E(uuid.New(), "Machine", "Read by an independent Actor"), "summary": "machine test"}, http.StatusCreated)

	for _, actorType := range []string{"ai", "import"} {
		t.Run(actorType, func(t *testing.T) {
			machine := requestE2EMapHeaders(t, admin, http.MethodPost, baseURL+"/api/v1/machine-actors",
				map[string]any{"actor_type": actorType, "display_name": "Agent " + actorType},
				http.Header{"Idempotency-Key": []string{uuid.NewString()}}, http.StatusCreated)
			machineID := e2eUUID(t, machine, "actor_id")
			if e2eUUID(t, machine, "owner_actor_id") != adminID {
				t.Fatal("machine lost account ownership")
			}
			requestE2EMap(t, other, http.MethodPost, baseURL+"/api/v1/auth/cli/codes",
				map[string]any{"actor_id": machineID, "name": "stolen", "token_ttl_days": 30}, http.StatusForbidden)
			choices := requestE2EMap(t, other, http.MethodGet, baseURL+"/api/v1/auth/cli/actors", nil, http.StatusOK)
			if len(choices["items"].([]any)) != 0 {
				t.Fatal("machine identity leaked to another account")
			}
			code := requestE2EMap(t, admin, http.MethodPost, baseURL+"/api/v1/auth/cli/codes",
				map[string]any{"actor_id": machineID, "name": "Agent CLI", "token_ttl_days": 30}, http.StatusCreated)
			app, err := wikicli.New("machine-e2e")
			if err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(t.TempDir(), "cli.json")
			exchanged, exitCode := app.Execute(context.Background(), wikicli.Input{
				Action: "auth.exchange", BaseURL: baseURL, ConfigPath: configPath, Code: code["code"].(string),
			})
			if !exchanged.OK || exitCode != 0 {
				t.Fatalf("CLI exchange failed: %v", exchanged.Error)
			}
			runCLI := func(input wikicli.Input, want int) map[string]any {
				t.Helper()
				input.ConfigPath = configPath
				result, _ := app.Execute(context.Background(), input)
				if result.Meta == nil || result.Meta.HTTPStatus != want {
					t.Fatalf("CLI %s %s: metadata=%+v error=%+v", input.Action, input.OperationID, result.Meta, result.Error)
				}
				if want >= 400 {
					if result.OK {
						t.Fatal("CLI accepted a forbidden operation")
					}
					return nil
				}
				if !result.OK {
					t.Fatalf("CLI operation failed: %v", result.Error)
				}
				return result.Data.(map[string]any)
			}
			identity := runCLI(wikicli.Input{Action: "auth.status"}, http.StatusOK)
			if e2eUUID(t, identity, "actor_id") != machineID || identity["actor_type"] != actorType {
				t.Fatal("CLI authenticated as its human owner")
			}
			local, _ := app.Execute(context.Background(), wikicli.Input{Action: "config.show", ConfigPath: configPath})
			if local.Data.(map[string]any)["actor_type"] != actorType || local.Data.(map[string]any)["wiki_id"] != wikiID.String() {
				t.Fatal("CLI config lacks machine identity metadata")
			}
			runCLI(wikicli.Input{Action: "operation.call", OperationID: "getPageByID", Path: map[string]any{"id": pageID.String()}}, http.StatusOK)
			runCLI(wikicli.Input{Action: "operation.call", OperationID: "createPage", Body: json.RawMessage(`{"namespace":"main","title":"Denied machine write"}`)}, http.StatusForbidden)
			runCLI(wikicli.Input{Action: "operation.call", OperationID: "createCLIAuthCode", Body: json.RawMessage(`{"name":"Nested","token_ttl_days":30}`)}, http.StatusForbidden)
			proposal := runCLI(wikicli.Input{
				Action: "operation.call", OperationID: "createProposal", Headers: map[string]any{"Idempotency-Key": uuid.NewString()},
				Body: json.RawMessage(`{"target_type":"entity","base_state_version":0,"risk_level":"medium"}`),
			}, http.StatusCreated)
			proposalID := e2eUUID(t, proposal, "id")
			if e2eUUID(t, proposal, "created_by") != machineID {
				t.Fatal("proposal did not record machine authorship")
			}
			entityID := uuid.New()
			operation, _ := json.Marshal(map[string]any{
				"schema_version": 1, "operation_type": "create_entity",
				"base":          map[string]any{"state_version": 0},
				"target":        map[string]any{"wiki_id": wikiID, "entity_id": entityID},
				"expected_hash": nil, "evidence": []any{},
				"risk": map[string]any{"level": "medium", "reasons": []string{"human review"}},
				"payload": map[string]any{
					"type_key": "concept", "canonical_key": "machine-" + actorType + "-" + runID,
					"labels": []any{map[string]any{"language": "und", "label": "Machine concept", "is_primary": true}},
				},
			})
			runCLI(wikicli.Input{Action: "operation.call", OperationID: "addProposalOperation", Path: map[string]any{"id": proposalID.String()}, Body: operation}, http.StatusCreated)
			submitted := runCLI(wikicli.Input{Action: "operation.call", OperationID: "submitProposal", Path: map[string]any{"id": proposalID.String()}}, http.StatusOK)
			if submitted["proposal"].(map[string]any)["status"] != "in_review" ||
				submitted["decision"].(map[string]any)["ai_trust_level"] != "untrusted" {
				t.Fatal("machine proposal bypassed trust or human review")
			}
			taskID := e2eUUID(t, submitted["review_task"].(map[string]any), "id")
			runCLI(wikicli.Input{Action: "operation.call", OperationID: "decideReviewTask", Path: map[string]any{"id": taskID.String()}, Body: json.RawMessage(`{"approve":true,"reason":"machine self approval"}`)}, http.StatusForbidden)
			requestE2EMap(t, admin, http.MethodPost, fmt.Sprintf("%s/api/v1/review-tasks/%s/decision", baseURL, taskID),
				map[string]any{"approve": true, "reason": "Human reviewed machine proposal"}, http.StatusOK)
			runCLI(wikicli.Input{Action: "operation.call", OperationID: "applyProposal", Path: map[string]any{"id": proposalID.String()}}, http.StatusForbidden)
			requestE2EMap(t, admin, http.MethodPost, fmt.Sprintf("%s/api/v1/proposals/%s/apply", baseURL, proposalID), map[string]any{}, http.StatusOK)
			runCLI(wikicli.Input{Action: "operation.call", OperationID: "getEntity", Path: map[string]any{"id": entityID.String()}}, http.StatusOK)
			requestE2EMap(t, admin, http.MethodPut, fmt.Sprintf("%s/api/v1/machine-actors/%s/status", baseURL, machineID),
				map[string]any{"status": "disabled"}, http.StatusOK)
			runCLI(wikicli.Input{Action: "auth.status"}, http.StatusUnauthorized)
		})
	}
}
