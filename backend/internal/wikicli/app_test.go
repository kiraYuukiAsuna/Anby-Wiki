package wikicli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anby/wiki/backend/internal/collaboration"
	"github.com/coder/websocket"
	"github.com/google/uuid"
)

func TestDecodeInputRejectsUnknownFields(t *testing.T) {
	_, err := DecodeInput(strings.NewReader(
		`{"action":"version","unexpected":true}`,
	))
	if err == nil {
		t.Fatal("unknown input field was accepted")
	}
}

func TestAuthExchangeChecksConfigPathBeforeRemoteCall(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		requests.Add(1)
		http.NotFound(writer, request)
	}))
	defer server.Close()

	app := testApp(t)
	result, exitCode := app.Execute(context.Background(), Input{
		Action:     "auth.exchange",
		BaseURL:    server.URL,
		ConfigPath: filepath.Join(os.TempDir(), "anby-wiki-cli-test.json"),
		Code:       "anby_code_test",
	})
	if exitCode != 1 || result.OK || result.Error == nil ||
		result.Error.Code != "config_write_failed" {
		t.Fatalf("result=%#v exit=%d", result, exitCode)
	}
	if requests.Load() != 0 {
		t.Fatalf("auth exchange called remote server %d times", requests.Load())
	}
}

func TestLocalActionSurface(t *testing.T) {
	app := testApp(t)
	configPath := filepath.Join(t.TempDir(), "cli.json")
	if err := saveConfig(configPath, Config{
		BaseURL: "https://example.invalid",
		Token:   "anby_token_never_expose", TokenPrefix: "anby_token_n",
		ActorID: uuid.NewString(), DisplayName: "CLI Test",
	}); err != nil {
		t.Fatal(err)
	}
	for _, input := range []Input{
		{Action: "help"},
		{Action: "version"},
		{Action: "operations.list"},
		{Action: "operations.list", Tag: "knowledge", Search: "entity"},
		{Action: "operation.describe", OperationID: "createPage"},
		{Action: "config.show", ConfigPath: configPath},
	} {
		result, exitCode := app.Execute(context.Background(), input)
		if exitCode != 0 || !result.OK {
			t.Fatalf("action=%s result=%#v exit=%d",
				input.Action, result, exitCode)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "anby_token_never_expose") {
			t.Fatalf("action=%s exposed the configured token", input.Action)
		}
		if input.Action == "operations.list" && input.Tag == "" {
			data := result.Data.(map[string]any)
			if data["count"] != 149 {
				t.Fatalf("operation count=%v want=149", data["count"])
			}
		}
	}

	result, exitCode := app.Execute(context.Background(), Input{Action: "missing"})
	if exitCode != 2 || result.OK || result.Error == nil ||
		result.Error.Code != "unknown_action" {
		t.Fatalf("unknown action result=%#v exit=%d", result, exitCode)
	}
}

func TestOperationCallValidatesAndUsesBearerToken(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		requests.Add(1)
		if request.URL.Path != "/api/v1/pages" {
			t.Errorf("path=%s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer anby_token_test" {
			t.Errorf("authorization=%q", request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-ID", "cli-test")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{
			"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
			"wiki_id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a00",
			"namespace_id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a02",
			"namespace":"main",
			"normalized_title":"cli-test",
			"display_title":"CLI Test",
			"language":"en",
			"content_model":"block-v1",
			"status":"active",
			"current_revision_id":null,
			"created_by":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a03",
			"created_at":"2026-08-18T00:00:00Z",
			"updated_at":"2026-08-18T00:00:00Z"
		}`))
	}))
	defer server.Close()

	app := testApp(t)
	configPath := filepath.Join(t.TempDir(), "cli.json")
	if err := saveConfig(configPath, Config{
		BaseURL: server.URL, Token: "anby_token_test",
	}); err != nil {
		t.Fatal(err)
	}
	result, exitCode := app.Execute(context.Background(), Input{
		Action: "operation.call", OperationID: "createPage",
		ConfigPath: configPath,
		Body: json.RawMessage(`{
			"namespace":"main",
			"title":"CLI Test",
			"language":"en",
			"content_model":"block-v1"
		}`),
	})
	if exitCode != 0 || !result.OK {
		t.Fatalf("error=%#v result=%#v exit=%d", result.Error, result, exitCode)
	}
	if result.Meta == nil || result.Meta.HTTPStatus != http.StatusCreated ||
		result.Meta.RequestID != "cli-test" {
		t.Fatalf("unexpected metadata: %#v", result.Meta)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests=%d want=1", requests.Load())
	}

	invalid, invalidExit := app.Execute(context.Background(), Input{
		Action: "operation.call", OperationID: "createPage",
		ConfigPath: configPath,
		Body:       json.RawMessage(`{"namespace":"main","title":"","extra":true}`),
	})
	if invalidExit != 2 || invalid.OK || invalid.Error == nil ||
		invalid.Error.Code != "validation_failed" {
		t.Fatalf("invalid result=%#v exit=%d", invalid, invalidExit)
	}
	if requests.Load() != 1 {
		t.Fatal("invalid request reached the server")
	}

	for _, test := range []struct {
		name  string
		input Input
		code  string
	}{
		{
			name: "missing body",
			input: Input{
				Action: "operation.call", OperationID: "createPage",
				ConfigPath: configPath,
			},
			code: "validation_failed",
		},
		{
			name: "invalid timeout",
			input: Input{
				Action: "operation.call", OperationID: "getHealthz",
				ConfigPath: configPath, TimeoutSeconds: 601,
			},
			code: "validation_failed",
		},
		{
			name: "unknown operation",
			input: Input{
				Action: "operation.call", OperationID: "missing",
				ConfigPath: configPath,
			},
			code: "operation_not_found",
		},
		{
			name: "auth status invalid timeout",
			input: Input{
				Action: "auth.status", ConfigPath: configPath,
				TimeoutSeconds: 601,
			},
			code: "validation_failed",
		},
		{
			name: "logout invalid timeout",
			input: Input{
				Action: "auth.logout", ConfigPath: configPath,
				TimeoutSeconds: 601,
			},
			code: "validation_failed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, exitCode := app.Execute(context.Background(), test.input)
			if exitCode != 2 || result.OK || result.Error == nil ||
				result.Error.Code != test.code {
				t.Fatalf("result=%#v exit=%d", result, exitCode)
			}
		})
	}
	if requests.Load() != 1 {
		t.Fatal("locally invalid request reached the server")
	}
	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if config.Token != "anby_token_test" {
		t.Fatal("invalid logout input removed the configured token")
	}
}

func TestAuthExchangePersistsTokenWithoutReturningSecret(t *testing.T) {
	expires := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/api/v1/auth/cli/exchange":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{
				"token":"anby_token_secret",
				"actor_id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
				"display_name":"CLI Test",
				"token_info":{
					"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a02",
					"name":"test-agent",
					"token_prefix":"anby_token_s",
					"status":"active",
					"expires_at":"` + expires.Format(time.RFC3339) + `",
					"created_at":"2026-08-18T00:00:00Z"
				}
			}`))
		case "/api/v1/auth/session":
			if request.Header.Get("Authorization") !=
				"Bearer anby_token_secret" {
				t.Errorf("authorization=%q",
					request.Header.Get("Authorization"))
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{
				"actor_id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
				"actor_type":"human",
				"display_name":"CLI Test",
				"method":"cli_token"
			}`))
		case "/api/v1/auth/cli/token":
			if request.Header.Get("Authorization") !=
				"Bearer anby_token_secret" {
				t.Errorf("authorization=%q",
					request.Header.Get("Authorization"))
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("path=%s", request.URL.Path)
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	app := testApp(t)
	configPath := filepath.Join(t.TempDir(), "cli.json")
	result, exitCode := app.Execute(context.Background(), Input{
		Action: "auth.exchange", BaseURL: server.URL,
		ConfigPath: configPath, Code: "anby_code_test",
	})
	if exitCode != 0 || !result.OK {
		t.Fatalf("result=%#v exit=%d", result, exitCode)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "anby_token_secret") {
		t.Fatal("persisted exchange result exposed plaintext token")
	}
	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if config.Token != "anby_token_secret" || config.BaseURL != server.URL {
		t.Fatalf("unexpected config: %#v", config)
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode=%o want=600", info.Mode().Perm())
	}

	status, statusExit := app.Execute(context.Background(), Input{
		Action: "auth.status", ConfigPath: configPath,
	})
	if statusExit != 0 || !status.OK || status.Meta == nil ||
		status.Meta.HTTPStatus != http.StatusOK {
		t.Fatalf("status=%#v exit=%d", status, statusExit)
	}
	shown, shownExit := app.Execute(context.Background(), Input{
		Action: "config.show", ConfigPath: configPath,
	})
	if shownExit != 0 || !shown.OK {
		t.Fatalf("shown=%#v exit=%d", shown, shownExit)
	}
	shownJSON, _ := json.Marshal(shown)
	if strings.Contains(string(shownJSON), "anby_token_secret") {
		t.Fatal("config.show exposed the plaintext token")
	}
	logout, logoutExit := app.Execute(context.Background(), Input{
		Action: "auth.logout", ConfigPath: configPath,
	})
	if logoutExit != 0 || !logout.OK || logout.Meta == nil ||
		logout.Meta.HTTPStatus != http.StatusNoContent {
		t.Fatalf("logout=%#v exit=%d", logout, logoutExit)
	}
	config, err = loadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if config.Token != "" || config.TokenPrefix != "" {
		t.Fatal("auth.logout did not remove local credentials")
	}
}

func TestCollaborationRunUsesBearerAndJSONBase64(t *testing.T) {
	pageID := uuid.New()
	documentID := uuid.New()
	updateID := uuid.New()
	update := []byte("opaque-yjs-update")
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.Header.Get("Authorization") != "Bearer anby_token_test" {
			t.Errorf("authorization=%q", request.Header.Get("Authorization"))
		}
		conn, err := websocket.Accept(writer, request, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer conn.CloseNow()
		ctx := request.Context()
		hello, _ := json.Marshal(map[string]any{
			"type": "hello", "document_id": documentID,
			"latest_sequence": 0,
		})
		ready, _ := json.Marshal(map[string]any{
			"type": "ready", "latest_sequence": 0,
		})
		if err := conn.Write(ctx, websocket.MessageText, hello); err != nil {
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, ready); err != nil {
			return
		}
		messageType, raw, err := conn.Read(ctx)
		if err != nil || messageType != websocket.MessageBinary {
			t.Errorf("read update: %v type=%v", err, messageType)
			return
		}
		gotID, gotUpdate, err := collaboration.DecodeClientUpdate(raw)
		if err != nil || gotID != updateID || string(gotUpdate) != string(update) {
			t.Errorf("update id=%s value=%q err=%v", gotID, gotUpdate, err)
			return
		}
		frame, _ := collaboration.EncodeServerFrame(
			collaboration.FrameUpdate, 1, update,
		)
		_ = conn.Write(ctx, websocket.MessageBinary, frame)
	}))
	defer server.Close()

	app := testApp(t)
	configPath := filepath.Join(t.TempDir(), "cli.json")
	if err := saveConfig(configPath, Config{
		BaseURL: server.URL, Token: "anby_token_test",
	}); err != nil {
		t.Fatal(err)
	}
	result, exitCode := app.Execute(context.Background(), Input{
		Action: "collaboration.run", ConfigPath: configPath,
		PageID: pageID.String(), ClientID: uuid.NewString(),
		Messages: []CollaborationMessage{{
			Type: "update", UpdateID: updateID.String(),
			DataBase64: base64.StdEncoding.EncodeToString(update),
		}},
	})
	if exitCode != 0 || !result.OK {
		t.Fatalf("error=%#v result=%#v", result.Error, result)
	}
	value, ok := result.Data.(collaborationResult)
	if !ok || value.DocumentID != documentID ||
		value.LatestSequence != 1 || len(value.Acknowledgments) != 1 {
		t.Fatalf("unexpected collaboration result: %#v", result.Data)
	}
}

func testApp(t *testing.T) *App {
	t.Helper()
	app, err := New("test")
	if err != nil {
		t.Fatal(err)
	}
	app.getenv = func(string) string { return "" }
	return app
}
