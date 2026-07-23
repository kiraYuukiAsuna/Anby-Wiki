package governance

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	OperationSchemaURL = "https://anby.wiki/schemas/proposal-operation/v1/operation.schema.json"
	OperationVersion   = 1

	OpCreatePage                = "create_page"
	OpRenamePage                = "rename_page"
	OpCreateRedirect            = "create_redirect"
	OpInsertBlock               = "insert_block"
	OpDeleteBlock               = "delete_block"
	OpMoveBlock                 = "move_block"
	OpReplaceBlock              = "replace_block"
	OpInsertPageReference       = "insert_page_reference"
	OpRetargetPageReference     = "retarget_page_reference"
	OpInsertEntityReference     = "insert_entity_reference"
	OpRetargetEntityReference   = "retarget_entity_reference"
	OpInsertClaimReference      = "insert_claim_reference"
	OpRetargetClaimReference    = "retarget_claim_reference"
	OpInsertCitationReference   = "insert_citation_reference"
	OpRetargetCitationReference = "retarget_citation_reference"
	OpRetargetExternalLink      = "retarget_external_link"
	OpCreateEntity              = "create_entity"
	OpCreateClaim               = "create_claim"
	OpSupersedeClaim            = "supersede_claim"
	OpAddClaimSource            = "add_claim_source"
)

//go:embed schema/operation.schema.json
var operationSchemaJSON []byte

var compiledOperationSchema = mustCompileOperationSchema()

type OperationBase struct {
	RevisionID   *uuid.UUID `json:"revision_id,omitempty"`
	StateVersion *int       `json:"state_version,omitempty"`
}

// OperationTarget 是所有 v1 操作稳定目标字段的并集；具体必填组合由 JSON Schema
// 的 discriminator 分支约束。
type OperationTarget struct {
	WikiID             *uuid.UUID `json:"wiki_id,omitempty"`
	NamespaceID        *uuid.UUID `json:"namespace_id,omitempty"`
	PageID             *uuid.UUID `json:"page_id,omitempty"`
	BlockID            *string    `json:"block_id,omitempty"`
	NodeID             *string    `json:"node_id,omitempty"`
	EntityID           *uuid.UUID `json:"entity_id,omitempty"`
	ClaimID            *uuid.UUID `json:"claim_id,omitempty"`
	CitationID         *uuid.UUID `json:"citation_id,omitempty"`
	ExternalResourceID *uuid.UUID `json:"external_resource_id,omitempty"`
}

type OperationEvidence struct {
	CitationID    *uuid.UUID `json:"citation_id,omitempty"`
	SourceChunkID *uuid.UUID `json:"source_chunk_id,omitempty"`
	Note          string     `json:"note,omitempty"`
}

type OperationRisk struct {
	Level   string   `json:"level"`
	Reasons []string `json:"reasons"`
}

// OperationV1 是语言绑定。Payload 保持 JSON object，由每种 Operation 的 Patch
// Engine 解码成自己的强类型参数；ParseOperationV1 先执行权威 Schema 校验。
type OperationV1 struct {
	SchemaVersion int                 `json:"schema_version"`
	OperationType string              `json:"operation_type"`
	Base          OperationBase       `json:"base"`
	Target        OperationTarget     `json:"target"`
	ExpectedHash  *string             `json:"expected_hash"`
	Evidence      []OperationEvidence `json:"evidence"`
	Risk          OperationRisk       `json:"risk"`
	Payload       json.RawMessage     `json:"payload"`
}

func mustCompileOperationSchema() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(operationSchemaJSON))
	if err != nil {
		panic(fmt.Sprintf("governance: 解析 Operation Schema 失败: %v", err))
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource(OperationSchemaURL, doc); err != nil {
		panic(fmt.Sprintf("governance: 注册 Operation Schema 失败: %v", err))
	}
	sch, err := c.Compile(OperationSchemaURL)
	if err != nil {
		panic(fmt.Sprintf("governance: 编译 Operation Schema 失败: %v", err))
	}
	return sch
}

func ValidateOperationJSON(raw []byte) error {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("%w: JSON 解码失败: %v", ErrInvalidOperation, err)
	}
	if err := compiledOperationSchema.Validate(instance); err != nil {
		return fmt.Errorf("%w: Schema 校验失败: %v", ErrInvalidOperation, err)
	}
	return nil
}

func ParseOperationV1(raw []byte) (*OperationV1, error) {
	if err := ValidateOperationJSON(raw); err != nil {
		return nil, err
	}
	var op OperationV1
	if err := json.Unmarshal(raw, &op); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidOperation, err)
	}
	return &op, nil
}

// AddOperationV1 校验权威契约后，把 envelope 各字段无损拆入 proposal_operation。
func (s *Service) AddOperationV1(ctx context.Context, proposalID uuid.UUID, raw []byte) (*OperationRecord, error) {
	op, err := ParseOperationV1(raw)
	if err != nil {
		return nil, err
	}
	base, _ := json.Marshal(op.Base)
	target, _ := json.Marshal(op.Target)
	evidence, _ := json.Marshal(op.Evidence)
	risk, _ := json.Marshal(op.Risk)
	return s.AddOperation(ctx, AddOperationParams{
		ProposalID: proposalID, SchemaVersion: op.SchemaVersion, OperationType: op.OperationType,
		TargetPageID: op.Target.PageID, TargetBlockID: op.Target.BlockID,
		TargetNodeID: op.Target.NodeID, TargetEntityID: op.Target.EntityID,
		TargetClaimID: op.Target.ClaimID, ExpectedHash: op.ExpectedHash,
		Target: target, Base: base, Evidence: evidence, Risk: risk, Payload: op.Payload,
	})
}
