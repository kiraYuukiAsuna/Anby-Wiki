package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/collaboration"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestCollaborationWebSocketBroadcastPresenceAndReconnect(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	actorID := tdb.MakeActor(t, "human", "collaborator")
	pageID := tdb.MakePage(
		t, testkit.MainNamespaceID, "collaboration-socket", "Collaboration Socket", actorID,
	)
	if _, err := tdb.Pool.Exec(context.Background(), `INSERT INTO actor_role
		(actor_id,role_id,wiki_id)
		SELECT $1,id,$2 FROM role WHERE role_key='editor'`,
		actorID, testkit.DefaultWikiID); err != nil {
		t.Fatal(err)
	}
	txm := db.NewTxManager(tdb.Pool)
	authService := authdomain.NewService(tdb.Pool, txm, id.NewGenerator(), time.Hour)
	authenticator := authdomain.NewAuthenticator(authService, "anby_session", true)
	hub := collaboration.NewHub()
	api := NewCollaborationAPI(
		collaboration.NewService(tdb.Pool, txm, id.NewGenerator()),
		governance.NewAuthorizationService(tdb.Pool),
		hub,
		testkit.DefaultWikiID,
	)
	router := NewRouter(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Deps{Environment: "development", Authenticator: authenticator},
		nil, nil, nil, nil, nil, api,
	)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	socketURL := strings.Replace(server.URL, "http://", "ws://", 1) +
		"/api/v1/pages/" + pageID.String() + "/collaboration"
	if _, response, err := websocket.Dial(
		context.Background(), socketURL+"?client_id="+uuid.NewString(), nil,
	); err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous dial response=%v err=%v", response, err)
	}

	first := dialCollaboration(t, socketURL, actorID, uuid.New())
	defer first.CloseNow()
	second := dialCollaboration(t, socketURL, actorID, uuid.New())
	defer second.CloseNow()
	readHello(t, first)
	readHello(t, second)
	readReady(t, first)
	readReady(t, second)

	updateID := uuid.New()
	payload := []byte("yjs-update")
	clientFrame := append(updateID[:], payload...)
	writeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := first.Write(writeCtx, websocket.MessageBinary, clientFrame); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	for _, conn := range []*websocket.Conn{first, second} {
		messageType, value := readSocket(t, conn)
		if messageType != websocket.MessageBinary {
			t.Fatalf("message type=%v", messageType)
		}
		kind, sequence, got, err := collaboration.DecodeServerFrame(value)
		if err != nil {
			t.Fatal(err)
		}
		if kind != collaboration.FrameUpdate || sequence != 1 || string(got) != string(payload) {
			t.Fatalf("kind=%d sequence=%d payload=%q", kind, sequence, got)
		}
	}

	writeCtx, cancel = context.WithTimeout(context.Background(), time.Second)
	if err := first.Write(
		writeCtx,
		websocket.MessageText,
		[]byte(`{"type":"presence","cursor":{"block_id":"block-1","offset":3}}`),
	); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	for _, conn := range []*websocket.Conn{first, second} {
		messageType, value := readSocket(t, conn)
		if messageType != websocket.MessageText {
			t.Fatalf("message type=%v", messageType)
		}
		var message struct {
			Type    string    `json:"type"`
			ActorID uuid.UUID `json:"actor_id"`
		}
		if err := json.Unmarshal(value, &message); err != nil {
			t.Fatal(err)
		}
		if message.Type != "presence" || message.ActorID != actorID {
			t.Fatalf("presence=%s", value)
		}
	}

	reconnected := dialCollaboration(t, socketURL, actorID, uuid.New())
	defer reconnected.CloseNow()
	readHello(t, reconnected)
	messageType, value := readSocket(t, reconnected)
	if messageType != websocket.MessageBinary {
		t.Fatalf("recovery message type=%v", messageType)
	}
	kind, sequence, got, err := collaboration.DecodeServerFrame(value)
	if err != nil || kind != collaboration.FrameUpdate || sequence != 1 ||
		string(got) != string(payload) {
		t.Fatalf("recovery kind=%d sequence=%d payload=%q err=%v", kind, sequence, got, err)
	}
	readReady(t, reconnected)

	if _, err := tdb.Pool.Exec(context.Background(), `DELETE FROM actor_role WHERE actor_id=$1`, actorID); err != nil {
		t.Fatal(err)
	}
	nextUpdateID := uuid.New()
	writeCtx, cancel = context.WithTimeout(context.Background(), time.Second)
	err = reconnected.Write(
		writeCtx,
		websocket.MessageBinary,
		append(nextUpdateID[:], []byte("must-not-persist")...),
	)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, _, err = reconnected.Read(readCtx)
	readCancel()
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("permission revoke close status=%v err=%v", websocket.CloseStatus(err), err)
	}
	var updates int
	if err := tdb.Pool.QueryRow(context.Background(), `SELECT count(*)
		FROM working_document_update`).Scan(&updates); err != nil {
		t.Fatal(err)
	}
	if updates != 1 {
		t.Fatalf("revoked update persisted: count=%d", updates)
	}
}

func dialCollaboration(
	t *testing.T,
	socketURL string,
	actorID, clientID uuid.UUID,
) *websocket.Conn {
	t.Helper()
	headers := make(http.Header)
	headers.Set(authdomain.DevActorHeader, actorID.String())
	conn, response, err := websocket.Dial(
		context.Background(),
		socketURL+"?client_id="+clientID.String()+"&last_sequence=0",
		&websocket.DialOptions{HTTPHeader: headers},
	)
	if err != nil {
		t.Fatalf("dial response=%v err=%v", response, err)
	}
	return conn
}

func readHello(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	messageType, value := readSocket(t, conn)
	if messageType != websocket.MessageText {
		t.Fatalf("hello message type=%v", messageType)
	}
	var message struct {
		Type       string    `json:"type"`
		DocumentID uuid.UUID `json:"document_id"`
	}
	if err := json.Unmarshal(value, &message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "hello" || message.DocumentID == uuid.Nil {
		t.Fatalf("hello=%s", value)
	}
}

func readReady(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	messageType, value := readSocket(t, conn)
	if messageType != websocket.MessageText {
		t.Fatalf("ready message type=%v", messageType)
	}
	var message struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(value, &message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "ready" {
		t.Fatalf("ready=%s", value)
	}
}

func readSocket(t *testing.T, conn *websocket.Conn) (websocket.MessageType, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	messageType, value, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return messageType, value
}
