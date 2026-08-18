package governance

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
)

func TestIdentityConflictClassification(t *testing.T) {
	conflict := MergeConflict{
		ConflictType:  ConflictSemantic,
		ProposedValue: mustIdentityJSON(map[string]any{"kind": identityConflictPage}),
	}
	if !isIdentityConflict(conflict) {
		t.Fatal("page identity semantic conflict was not recognized")
	}
	conflict.ProposedValue = mustIdentityJSON(map[string]any{"kind": "other"})
	if isIdentityConflict(conflict) {
		t.Fatal("unrelated semantic conflict was classified as identity conflict")
	}
}

func TestBulkApplyErrorCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "governance identity", err: ErrIdentityConflict, want: BulkApplyErrorIdentityConflict},
		{name: "wrapped page identity", err: fmt.Errorf("apply: %w", page.ErrTitleConflict), want: BulkApplyErrorIdentityConflict},
		{name: "entity identity", err: knowledge.ErrDuplicateEntityKey, want: BulkApplyErrorIdentityConflict},
		{name: "merge", err: ErrMergeConflict, want: BulkApplyErrorMergeConflict},
		{name: "state", err: ErrInvalidTransition, want: BulkApplyErrorStateConflict},
		{name: "validation", err: ErrInvalidOperation, want: BulkApplyErrorValidation},
		{name: "permission", err: ErrPermissionDenied, want: BulkApplyErrorPermission},
		{name: "unknown", err: errors.New("boom"), want: BulkApplyErrorInternal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := bulkApplyErrorCode(test.err); got != test.want {
				t.Fatalf("bulkApplyErrorCode() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCreateEntityIdentityPayloadAcceptsSchemaLabels(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{
		"type_key":"concept",
		"canonical_key":"api-e2e",
		"labels":[{
			"language":"und",
			"label":"API E2E",
			"description":"isolated entity",
			"is_primary":true
		}]
	}`)
	var decoded createEntityIdentityPayload
	if err := decodeOperationPayload(payload, &decoded); err != nil {
		t.Fatalf("schema-valid create_entity payload was rejected: %v", err)
	}
	if len(decoded.Labels) != 1 ||
		decoded.Labels[0].Language != "und" ||
		decoded.Labels[0].Description != "isolated entity" {
		t.Fatalf("unexpected decoded payload: %#v", decoded)
	}
}
