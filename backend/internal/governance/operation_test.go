package governance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/testkit"
)

const operationContractDir = "../../../contracts/schemas/proposal-operation/v1"

func TestOperationSchemaCopyAndFixtures(t *testing.T) {
	authoritative, err := os.ReadFile(filepath.Join(operationContractDir, "operation.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := os.ReadFile("schema/operation.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(authoritative, embedded) {
		t.Fatal("Go 内嵌 Operation Schema 与 contracts 权威文件漂移")
	}

	valid, _ := filepath.Glob(filepath.Join(operationContractDir, "fixtures/valid/*.json"))
	if len(valid) == 0 {
		t.Fatal("缺少 valid fixture")
	}
	for _, path := range valid {
		t.Run("valid/"+filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			op, err := governance.ParseOperationV1(raw)
			if err != nil {
				t.Fatal(err)
			}
			if op.SchemaVersion != 1 || op.OperationType == "" {
				t.Fatalf("unexpected op: %+v", op)
			}
		})
	}

	invalid, _ := filepath.Glob(filepath.Join(operationContractDir, "fixtures/invalid/*.json"))
	for _, path := range invalid {
		t.Run("invalid/"+filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := governance.ParseOperationV1(raw); !errors.Is(err, governance.ErrInvalidOperation) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestAddOperationV1PersistsEnvelope(t *testing.T) {
	svc, tdb := newService(t)
	ctx := context.Background()
	ai := tdb.MakeActor(t, "ai", "contract agent")
	pageID := tdb.MakePage(t, testkit.MainNamespaceID, "contract-page", "Contract Page", ai)
	p, err := svc.CreateProposal(ctx, governance.CreateProposalParams{
		TargetType: governance.TargetPage, CreatedBy: ai, IdempotencyKey: "contract-op",
	})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(operationContractDir, "fixtures/valid/insert-block.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.ReplaceAll(raw,
		[]byte("00000000-0000-7000-8000-000000000502"), []byte(pageID.String()))
	op, err := svc.AddOperationV1(ctx, p.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	var base map[string]any
	var risk governance.OperationRisk
	if err := json.Unmarshal(op.Base, &base); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(op.Risk, &risk); err != nil {
		t.Fatal(err)
	}
	if base["revision_id"] == nil || risk.Level != governance.RiskLow || op.TargetPageID == nil {
		t.Fatalf("envelope 未无损持久化: base=%s risk=%s target=%v", op.Base, op.Risk, op.TargetPageID)
	}
}
