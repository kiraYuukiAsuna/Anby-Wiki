package governance

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestCreateEntityOperationRequiresPreallocatedEntityID(t *testing.T) {
	baseVersion := 0
	wikiID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"type_key": "concept", "canonical_key": "atomic-import",
		"labels": []map[string]any{{"language": "und", "label": "Atomic import", "is_primary": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	op := OperationV1{
		SchemaVersion: 1, OperationType: OpCreateEntity,
		Base:   OperationBase{StateVersion: &baseVersion},
		Target: OperationTarget{WikiID: &wikiID}, ExpectedHash: nil,
		Evidence: []OperationEvidence{},
		Risk:     OperationRisk{Level: RiskMedium, Reasons: []string{"review"}}, Payload: payload,
	}
	raw, _ := json.Marshal(op)
	if err := ValidateOperationJSON(raw); err == nil {
		t.Fatal("create_entity without a preallocated entity_id must be rejected")
	}

	entityID := uuid.New()
	op.Target.EntityID = &entityID
	raw, _ = json.Marshal(op)
	if err := ValidateOperationJSON(raw); err != nil {
		t.Fatalf("create_entity with stable target was rejected: %v", err)
	}
	params := operationParams(uuid.New(), &op)
	if params.TargetEntityID != nil {
		t.Fatal("planned create_entity ID must not populate the existing-entity FK index")
	}
	claimPayload := json.RawMessage(`{"property_key":"author","value":{"entity_id":"` + entityID.String() +
		`"},"origin_type":"import","citation_ids":[]}`)
	claim := OperationV1{
		SchemaVersion: 1, OperationType: OpCreateClaim,
		Base:     OperationBase{StateVersion: &baseVersion},
		Target:   OperationTarget{WikiID: &wikiID, EntityID: &entityID},
		Evidence: []OperationEvidence{}, Risk: OperationRisk{Level: RiskLow, Reasons: []string{"new"}},
		Payload: claimPayload,
	}
	if params := operationParams(uuid.New(), &claim); params.TargetEntityID != nil {
		t.Fatal("create_claim subject may be planned and must not populate the existing-entity FK index")
	}
}

func TestCreatePageOperationRequiresPreallocatedPageID(t *testing.T) {
	baseVersion := 0
	wikiID, namespaceID := uuid.New(), uuid.New()
	payload := json.RawMessage(`{"title":"Atomic page","language":"zh-Hans","content_model":"block-v1","initial_ast":{"type":"document","schema_version":1,"children":[]},"summary":"import"}`)
	op := OperationV1{
		SchemaVersion: 1, OperationType: OpCreatePage,
		Base:     OperationBase{StateVersion: &baseVersion},
		Target:   OperationTarget{WikiID: &wikiID, NamespaceID: &namespaceID},
		Evidence: []OperationEvidence{}, Risk: OperationRisk{Level: RiskMedium, Reasons: []string{"review"}},
		Payload: payload,
	}
	raw, _ := json.Marshal(op)
	if err := ValidateOperationJSON(raw); err == nil {
		t.Fatal("create_page without a preallocated page_id must be rejected")
	}

	pageID := uuid.New()
	op.Target.PageID = &pageID
	raw, _ = json.Marshal(op)
	if err := ValidateOperationJSON(raw); err != nil {
		t.Fatalf("create_page with stable target was rejected: %v", err)
	}
	if params := operationParams(uuid.New(), &op); params.TargetPageID != nil {
		t.Fatal("planned create_page ID must not populate the existing-page FK index")
	}
}
