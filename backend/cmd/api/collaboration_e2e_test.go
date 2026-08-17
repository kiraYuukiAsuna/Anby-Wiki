package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/collaboration"
)

const collaborationE2ETimeout = 10 * time.Second

type e2eAuthResult struct {
	ActorID uuid.UUID `json:"actor_id"`
}

type e2ePage struct {
	ID uuid.UUID `json:"id"`
}

type e2eHello struct {
	Type           string    `json:"type"`
	DocumentID     uuid.UUID `json:"document_id"`
	LatestSequence int64     `json:"latest_sequence"`
}

type e2ePresence struct {
	Type    string         `json:"type"`
	ActorID uuid.UUID      `json:"actor_id"`
	Cursor  map[string]any `json:"cursor"`
}

type e2eRecovery struct {
	Hello   e2eHello
	Updates []e2eServerUpdate
	Ready   int64
}

type e2eServerUpdate struct {
	Sequence int64
	Payload  []byte
}

func TestCollaborationE2E(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("COLLABORATION_E2E_BASE_URL"), "/")
	password := os.Getenv("COLLABORATION_E2E_PASSWORD")
	if baseURL == "" || password == "" {
		t.Skip("set COLLABORATION_E2E_BASE_URL and COLLABORATION_E2E_PASSWORD")
	}

	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	firstUsername := "collab_e2e_a_" + runID
	secondUsername := "collab_e2e_b_" + runID
	firstClient, firstActor := registerE2EUser(
		t, baseURL, firstUsername, firstUsername+"@example.invalid", password,
	)
	secondClient, secondActor := registerE2EUser(
		t, baseURL, secondUsername, secondUsername+"@example.invalid", password,
	)

	page := postE2EJSON[e2ePage](t, firstClient, baseURL+"/api/v1/pages", map[string]any{
		"namespace":     "main",
		"title":         "Collaboration E2E " + runID,
		"language":      "en",
		"content_model": "block-v1",
	}, http.StatusCreated)

	firstClientID := uuid.New()
	secondClientID := uuid.New()
	firstConn, firstRecovery := dialE2ECollaboration(
		t, firstClient, baseURL, page.ID, firstClientID, 0,
	)
	defer firstConn.CloseNow()
	secondConn, secondRecovery := dialE2ECollaboration(
		t, secondClient, baseURL, page.ID, secondClientID, 0,
	)
	defer secondConn.CloseNow()

	if firstRecovery.Hello.DocumentID == uuid.Nil ||
		firstRecovery.Hello.DocumentID != secondRecovery.Hello.DocumentID {
		t.Fatalf("clients joined different working documents: %s / %s",
			firstRecovery.Hello.DocumentID, secondRecovery.Hello.DocumentID)
	}
	if firstRecovery.Ready != 0 || secondRecovery.Ready != 0 {
		t.Fatalf("new working document has non-zero sequence: %d / %d",
			firstRecovery.Ready, secondRecovery.Ready)
	}

	writeE2EJSON(t, firstConn, map[string]any{
		"type": "presence",
		"cursor": map[string]any{
			"block_id":  "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
			"selection": "text",
		},
	})
	presence := readE2EPresence(t, secondConn)
	if presence.ActorID != firstActor {
		t.Fatalf("presence actor = %s, want %s", presence.ActorID, firstActor)
	}
	if presence.ActorID == secondActor || presence.Cursor["selection"] != "text" {
		t.Fatalf("unexpected presence: %#v", presence)
	}

	updateID := uuid.New()
	updatePayload := []byte("e2e-yjs-update-v1")
	updateFrame := encodeE2EClientUpdate(updateID, updatePayload)
	writeE2EBinary(t, firstConn, updateFrame)
	assertE2EUpdate(t, readE2EUpdate(t, firstConn), 1, updatePayload)
	assertE2EUpdate(t, readE2EUpdate(t, secondConn), 1, updatePayload)

	// Reusing the same update id and bytes is idempotent and returns sequence 1.
	writeE2EBinary(t, firstConn, updateFrame)
	assertE2EUpdate(t, readE2EUpdate(t, firstConn), 1, updatePayload)
	assertE2EUpdate(t, readE2EUpdate(t, secondConn), 1, updatePayload)

	emptyAST := map[string]any{
		"type":           "document",
		"schema_version": 1,
		"children":       []any{},
	}
	publishURL := fmt.Sprintf("%s/api/v1/pages/%s/revisions", baseURL, page.ID)
	stale := postE2ERaw(t, firstClient, publishURL, map[string]any{
		"working_document_id": firstRecovery.Hello.DocumentID,
		"expected_sequence":   0,
		"ast":                 emptyAST,
		"summary":             "stale collaboration publish",
	})
	defer stale.Body.Close()
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale publish status = %d, want 409: %s",
			stale.StatusCode, readE2EBody(stale.Body))
	}

	published := postE2ERaw(t, firstClient, publishURL, map[string]any{
		"working_document_id": firstRecovery.Hello.DocumentID,
		"expected_sequence":   1,
		"ast":                 emptyAST,
		"summary":             "collaboration e2e publish",
	})
	defer published.Body.Close()
	if published.StatusCode != http.StatusCreated {
		t.Fatalf("current publish status = %d, want 201: %s",
			published.StatusCode, readE2EBody(published.Body))
	}

	if err := secondConn.Close(websocket.StatusNormalClosure, "e2e reconnect"); err != nil {
		t.Fatalf("close second collaboration socket: %v", err)
	}
	reconnected, recovered := dialE2ECollaboration(
		t, secondClient, baseURL, page.ID, uuid.New(), 0,
	)
	defer reconnected.CloseNow()
	if recovered.Ready != 1 || len(recovered.Updates) != 1 {
		t.Fatalf("recovery ready=%d updates=%d, want sequence 1 with one update",
			recovered.Ready, len(recovered.Updates))
	}
	assertE2EUpdate(t, recovered.Updates[0], 1, updatePayload)
}

func registerE2EUser(
	t *testing.T,
	baseURL, username, email, password string,
) (*http.Client, uuid.UUID) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: collaborationE2ETimeout}
	result := postE2EJSON[e2eAuthResult](
		t, client, baseURL+"/api/v1/auth/register", map[string]any{
			"username":     username,
			"email":        email,
			"password":     password,
			"display_name": username,
		}, http.StatusCreated,
	)
	return client, result.ActorID
}

func postE2EJSON[T any](
	t *testing.T,
	client *http.Client,
	endpoint string,
	body any,
	wantStatus int,
) T {
	t.Helper()
	response := postE2ERaw(t, client, endpoint, body)
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("POST %s status=%d want=%d: %s",
			endpoint, response.StatusCode, wantStatus, readE2EBody(response.Body))
	}
	var result T
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode %s response: %v", endpoint, err)
	}
	return result
}

func postE2ERaw(
	t *testing.T,
	client *http.Client,
	endpoint string,
	body any,
) *http.Response {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(
		http.MethodPost, endpoint, bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	return response
}

func dialE2ECollaboration(
	t *testing.T,
	client *http.Client,
	baseURL string,
	pageID, clientID uuid.UUID,
	lastSequence int64,
) (*websocket.Conn, e2eRecovery) {
	t.Helper()
	httpURL, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	scheme := "ws"
	if httpURL.Scheme == "https" {
		scheme = "wss"
	}
	socketURL := url.URL{
		Scheme: scheme,
		Host:   httpURL.Host,
		Path:   fmt.Sprintf("/api/v1/pages/%s/collaboration", pageID),
	}
	query := socketURL.Query()
	query.Set("client_id", clientID.String())
	query.Set("last_sequence", fmt.Sprintf("%d", lastSequence))
	socketURL.RawQuery = query.Encode()

	header := http.Header{}
	cookies := client.Jar.Cookies(httpURL)
	if len(cookies) == 0 {
		t.Fatal("authenticated client has no session cookie")
	}
	for _, cookie := range cookies {
		header.Add("Cookie", cookie.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), collaborationE2ETimeout)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, socketURL.String(), &websocket.DialOptions{
		HTTPHeader: header,
	})
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		status := 0
		var body string
		if response != nil {
			status = response.StatusCode
			body = readE2EBody(response.Body)
		}
		t.Fatalf("dial collaboration status=%d: %v %s", status, err, body)
	}
	return conn, readE2ERecovery(t, conn)
}

func readE2ERecovery(t *testing.T, conn *websocket.Conn) e2eRecovery {
	t.Helper()
	var recovery e2eRecovery
	for {
		messageType, payload := readE2EMessage(t, conn)
		if messageType == websocket.MessageBinary {
			recovery.Updates = append(
				recovery.Updates,
				decodeE2EUpdate(t, payload),
			)
			continue
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			t.Fatalf("decode collaboration envelope: %v", err)
		}
		switch envelope.Type {
		case "hello":
			if err := json.Unmarshal(payload, &recovery.Hello); err != nil {
				t.Fatalf("decode hello: %v", err)
			}
		case "ready":
			var ready struct {
				LatestSequence int64 `json:"latest_sequence"`
			}
			if err := json.Unmarshal(payload, &ready); err != nil {
				t.Fatalf("decode ready: %v", err)
			}
			recovery.Ready = ready.LatestSequence
			return recovery
		}
	}
}

func readE2EPresence(t *testing.T, conn *websocket.Conn) e2ePresence {
	t.Helper()
	messageType, payload := readE2EMessage(t, conn)
	if messageType != websocket.MessageText {
		t.Fatalf("presence message type = %v", messageType)
	}
	var presence e2ePresence
	if err := json.Unmarshal(payload, &presence); err != nil {
		t.Fatalf("decode presence: %v", err)
	}
	if presence.Type != "presence" {
		t.Fatalf("unexpected presence type %q", presence.Type)
	}
	return presence
}

func readE2EUpdate(t *testing.T, conn *websocket.Conn) e2eServerUpdate {
	t.Helper()
	messageType, payload := readE2EMessage(t, conn)
	if messageType != websocket.MessageBinary {
		t.Fatalf("update message type = %v", messageType)
	}
	return decodeE2EUpdate(t, payload)
}

func decodeE2EUpdate(t *testing.T, frame []byte) e2eServerUpdate {
	t.Helper()
	kind, sequence, payload, err := collaboration.DecodeServerFrame(frame)
	if err != nil {
		t.Fatalf("decode server update: %v", err)
	}
	if kind != collaboration.FrameUpdate {
		t.Fatalf("server frame kind = %d, want update", kind)
	}
	return e2eServerUpdate{Sequence: sequence, Payload: payload}
}

func writeE2EJSON(t *testing.T, conn *websocket.Conn, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeE2E(t, conn, websocket.MessageText, payload)
}

func writeE2EBinary(t *testing.T, conn *websocket.Conn, value []byte) {
	t.Helper()
	writeE2E(t, conn, websocket.MessageBinary, value)
}

func writeE2E(
	t *testing.T,
	conn *websocket.Conn,
	messageType websocket.MessageType,
	value []byte,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), collaborationE2ETimeout)
	defer cancel()
	if err := conn.Write(ctx, messageType, value); err != nil {
		t.Fatalf("write collaboration message: %v", err)
	}
}

func readE2EMessage(
	t *testing.T,
	conn *websocket.Conn,
) (websocket.MessageType, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), collaborationE2ETimeout)
	defer cancel()
	messageType, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read collaboration message: %v", err)
	}
	return messageType, payload
}

func encodeE2EClientUpdate(updateID uuid.UUID, payload []byte) []byte {
	frame := make([]byte, 16+len(payload))
	copy(frame[:16], updateID[:])
	copy(frame[16:], payload)
	return frame
}

func assertE2EUpdate(
	t *testing.T,
	update e2eServerUpdate,
	wantSequence int64,
	wantPayload []byte,
) {
	t.Helper()
	if update.Sequence != wantSequence || !bytes.Equal(update.Payload, wantPayload) {
		t.Fatalf("update sequence=%d payload=%q, want sequence=%d payload=%q",
			update.Sequence, update.Payload, wantSequence, wantPayload)
	}
}

func readE2EBody(body io.Reader) string {
	payload, _ := io.ReadAll(io.LimitReader(body, 8<<10))
	return string(payload)
}
