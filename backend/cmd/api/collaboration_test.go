package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	authdomain "github.com/anby/wiki/backend/internal/auth"
)

func TestPresenceMessageAddsAuthenticatedActor(t *testing.T) {
	t.Parallel()

	actorID := uuid.New()
	raw, err := presenceMessage(
		[]byte(`{"type":"presence","cursor":{"block_id":"block-1"}}`),
		authdomain.Principal{
			ActorID:     actorID,
			Username:    "alice",
			DisplayName: "Alice Chen",
		},
		uuid.New(),
	)
	if err != nil {
		t.Fatal(err)
	}
	var message struct {
		Type             string         `json:"type"`
		ActorID          uuid.UUID      `json:"actor_id"`
		ActorUsername    string         `json:"actor_username"`
		ActorDisplayName string         `json:"actor_display_name"`
		Cursor           map[string]any `json:"cursor"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "presence" || message.ActorID != actorID {
		t.Fatalf("unexpected presence envelope: %#v", message)
	}
	if message.ActorUsername != "alice" || message.ActorDisplayName != "Alice Chen" {
		t.Fatalf("unexpected actor label: %#v", message)
	}
	if message.Cursor["block_id"] != "block-1" {
		t.Fatalf("unexpected cursor: %#v", message.Cursor)
	}
}

func TestPresenceMessageRejectsInvalidShape(t *testing.T) {
	t.Parallel()

	if _, err := presenceMessage(
		[]byte(`{"type":"other","cursor":{}}`),
		authdomain.Principal{ActorID: uuid.New()},
		uuid.New(),
	); err == nil {
		t.Fatal("invalid presence type was accepted")
	}
}

func TestPresenceMessageRejectsInvalidCharacterPosition(t *testing.T) {
	t.Parallel()
	for _, cursor := range []string{
		`null`, `[]`, `"text"`,
		`{"version":2,"anchor":{},"head":{}}`,
		`{"version":1,"anchor":{"block_id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01","offset":-1},"head":{}}`,
		`{"version":1,"anchor":{"block_id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01","offset":0,"relative_position":"***"},"head":{}}`,
	} {
		if _, err := presenceMessage([]byte(`{"type":"presence","cursor":`+cursor+`}`),
			authdomain.Principal{ActorID: uuid.New()}, uuid.New()); err == nil {
			t.Fatalf("invalid cursor accepted: %s", cursor)
		}
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

func TestSnapshotMessage(t *testing.T) {
	t.Parallel()

	state := []byte("opaque-yjs-state")
	raw, err := json.Marshal(map[string]any{
		"type":           "snapshot",
		"up_to_sequence": 42,
		"state":          base64.StdEncoding.EncodeToString(state),
		"compact":        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sequence, decoded, compact, err := snapshotMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if sequence != 42 || string(decoded) != string(state) || !compact {
		t.Fatalf("unexpected snapshot: sequence=%d state=%q compact=%t",
			sequence, decoded, compact)
	}
}

func TestSnapshotMessageRejectsInvalidState(t *testing.T) {
	t.Parallel()

	if _, _, _, err := snapshotMessage(
		[]byte(`{"type":"snapshot","up_to_sequence":1,"state":"***","compact":true}`),
	); err == nil {
		t.Fatal("invalid base64 snapshot was accepted")
	}
}
