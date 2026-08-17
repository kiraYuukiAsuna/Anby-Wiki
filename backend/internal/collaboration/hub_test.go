package collaboration

import (
	"testing"

	"github.com/google/uuid"
)

func TestBroadcastExcept(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	documentID := uuid.New()
	sender := hub.Subscribe(documentID)
	peer := hub.Subscribe(documentID)
	defer sender.Close()
	defer peer.Close()

	hub.BroadcastExcept(
		documentID,
		HubMessage{Data: []byte(`{"type":"presence"}`)},
		sender,
	)

	select {
	case <-sender.Messages():
		t.Fatal("sender received its own presence")
	default:
	}

	select {
	case message := <-peer.Messages():
		if string(message.Data) != `{"type":"presence"}` {
			t.Fatalf("peer received %q", message.Data)
		}
	default:
		t.Fatal("peer did not receive presence")
	}
}
