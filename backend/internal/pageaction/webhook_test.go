package pageaction

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/page"
)

type fakePageReader struct {
	snapshot *page.ContentSnapshot
}

func (f fakePageReader) CurrentContent(
	context.Context,
	uuid.UUID,
) (*page.Revision, *page.ContentSnapshot, error) {
	return &page.Revision{ID: uuid.New()}, f.snapshot, nil
}

func TestExecuteWebhookRejectsPrivateAddress(t *testing.T) {
	blockID := uuid.NewString()
	document := &ast.Document{
		Type: "document", SchemaVersion: ast.SchemaVersion,
		Children: []*ast.Block{{
			ID: blockID, Type: ast.BlockButton, Label: "Run",
			Actions: []ast.ButtonAction{{
				Type:    ast.ButtonActionWebhook,
				URL:     "https://127.0.0.1/hook",
				Payload: json.RawMessage(`{"ok":true}`),
			}},
		}},
	}
	raw, err := ast.CanonicalJSON(document)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(fakePageReader{
		snapshot: &page.ContentSnapshot{AST: raw},
	})
	_, err = service.ExecuteWebhook(
		context.Background(), uuid.New(), blockID, 0, uuid.New(),
	)
	if !errors.Is(err, ErrWebhookRejected) {
		t.Fatalf("expected private webhook rejection, got %v", err)
	}
}

func TestPublicIPPolicy(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.2", "::1", "fc00::1"} {
		if isPublicIP(net.ParseIP(raw)) {
			t.Fatalf("private or local address accepted: %s", raw)
		}
	}
	if !isPublicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public address rejected")
	}
}
