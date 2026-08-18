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
	if invalidExit == 0 || invalid.OK {
		t.Fatalf("invalid result=%#v exit=%d", invalid, invalidExit)
	}
	if requests.Load() != 1 {
		t.Fatal("invalid request reached the server")
	}
}

func TestAuthExchangePersistsTokenWithoutReturningSecret(t *testing.T) {
	expires := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/api/v1/auth/cli/exchange" {
			t.Errorf("path=%s", request.URL.Path)
		}
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
