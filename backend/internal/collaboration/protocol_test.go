package collaboration_test

import (
	"bytes"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/collaboration"
)

func TestProtocolFrames(t *testing.T) {
	payload := []byte("opaque-yjs-update")
	frame, err := collaboration.EncodeServerFrame(collaboration.FrameUpdate, 42, payload)
	if err != nil {
		t.Fatal(err)
	}
	kind, sequence, decoded, err := collaboration.DecodeServerFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	if kind != collaboration.FrameUpdate || sequence != 42 || !bytes.Equal(decoded, payload) {
		t.Fatalf("kind=%d sequence=%d payload=%q", kind, sequence, decoded)
	}

	updateID := uuid.New()
	clientFrame := append(updateID[:], payload...)
	gotID, gotPayload, err := collaboration.DecodeClientUpdate(clientFrame)
	if err != nil {
		t.Fatal(err)
	}
	if gotID != updateID || !bytes.Equal(gotPayload, payload) {
		t.Fatalf("id=%s payload=%q", gotID, gotPayload)
	}
}

func TestHubBroadcastAndClose(t *testing.T) {
	hub := collaboration.NewHub()
	documentID := uuid.New()
	first := hub.Subscribe(documentID)
	second := hub.Subscribe(documentID)
	if hub.RoomSize(documentID) != 2 {
		t.Fatalf("room size=%d", hub.RoomSize(documentID))
	}
	message := collaboration.HubMessage{Binary: true, Data: []byte("update")}
	hub.Broadcast(documentID, message)
	for _, subscription := range []*collaboration.Subscription{first, second} {
		got := <-subscription.Messages()
		if !got.Binary || !bytes.Equal(got.Data, message.Data) {
			t.Fatalf("message=%+v", got)
		}
	}
	first.Close()
	second.Close()
	if hub.RoomSize(documentID) != 0 {
		t.Fatalf("room not removed: %d", hub.RoomSize(documentID))
	}
}

func TestHubDisconnectsSlowSubscriber(t *testing.T) {
	hub := collaboration.NewHub()
	documentID := uuid.New()
	subscription := hub.Subscribe(documentID)
	for index := 0; index < 100; index++ {
		hub.Broadcast(documentID, collaboration.HubMessage{Data: []byte{byte(index)}})
	}
	if hub.RoomSize(documentID) != 0 {
		t.Fatalf("slow subscriber remained in room: %d", hub.RoomSize(documentID))
	}
	for range subscription.Messages() {
	}
}
