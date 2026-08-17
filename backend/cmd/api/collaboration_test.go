package main

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestPresenceMessageAddsAuthenticatedActor(t *testing.T) {
	t.Parallel()

	actorID := uuid.New()
	raw, err := presenceMessage(
		[]byte(`{"type":"presence","cursor":{"block_id":"block-1"}}`),
		actorID,
	)
	if err != nil {
		t.Fatal(err)
	}
	var message struct {
		Type    string         `json:"type"`
		ActorID uuid.UUID      `json:"actor_id"`
		Cursor  map[string]any `json:"cursor"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "presence" || message.ActorID != actorID {
		t.Fatalf("unexpected presence envelope: %#v", message)
	}
	if message.Cursor["block_id"] != "block-1" {
		t.Fatalf("unexpected cursor: %#v", message.Cursor)
	}
}

func TestPresenceMessageRejectsInvalidShape(t *testing.T) {
	t.Parallel()

	if _, err := presenceMessage([]byte(`{"type":"other","cursor":{}}`), uuid.New()); err == nil {
		t.Fatal("invalid presence type was accepted")
	}
}

func TestValidWorkingDocumentCursor(t *testing.T) {
	t.Parallel()

	documentID := uuid.New()
	nilID := uuid.Nil
	zero := int64(0)
	negative := int64(-1)

	for _, test := range []struct {
		name       string
		documentID *uuid.UUID
		sequence   *int64
		want       bool
	}{
		{name: "ordinary publish", want: true},
		{name: "collaboration publish", documentID: &documentID, sequence: &zero, want: true},
		{name: "missing sequence", documentID: &documentID, want: false},
		{name: "missing document", sequence: &zero, want: false},
		{name: "nil document", documentID: &nilID, sequence: &zero, want: false},
		{name: "negative sequence", documentID: &documentID, sequence: &negative, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := validWorkingDocumentCursor(test.documentID, test.sequence); got != test.want {
				t.Fatalf("validWorkingDocumentCursor(%v, %v) = %t, want %t",
					test.documentID, test.sequence, got, test.want)
			}
		})
	}
}
