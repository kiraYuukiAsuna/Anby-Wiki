// Package ast 提供 Typed Block AST v1 的 Go 类型与 Schema 校验。
//
// 权威契约是 contracts/schemas/ast/v1/ast.schema.json（JSON Schema draft 2020-12），
// 本包内嵌其字节级副本（schema/ast.schema.json）用于运行时校验；
// 副本与原文件的一致性由单测与 scripts/check-ast-schema-sync.sh 保证。
package ast

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anby/wiki/backend/internal/platform/id"
)

//go:embed schema/ast.schema.json
var schemaJSON []byte

// SchemaURL 是 AST v1 Schema 的 $id。
const SchemaURL = "https://anby.wiki/schemas/ast/v1/ast.schema.json"

// SchemaVersion 是 AST v1 的 schema_version 常量。
const SchemaVersion = 1

// BlockType 是 v1 的 Block 类型判别值。
type BlockType string

const (
	BlockHeading     BlockType = "heading"
	BlockParagraph   BlockType = "paragraph"
	BlockBulletList  BlockType = "bullet_list"
	BlockOrderedList BlockType = "ordered_list"
	BlockListItem    BlockType = "list_item"
	BlockTable       BlockType = "table"
	BlockTableRow    BlockType = "table_row"
	BlockTableCell   BlockType = "table_cell"
	BlockCode        BlockType = "code"
	BlockQuote       BlockType = "quote"
	BlockCallout     BlockType = "callout"
	BlockComponent   BlockType = "component"
	BlockDivider     BlockType = "divider"
)

// CalloutKind 是 callout Block 的种类。
type CalloutKind string

const (
	CalloutInfo    CalloutKind = "info"
	CalloutWarning CalloutKind = "warning"
	CalloutDanger  CalloutKind = "danger"
)

// Mark 是 text 行内节点的修饰。
type Mark string

const (
	MarkBold          Mark = "bold"
	MarkItalic        Mark = "italic"
	MarkStrikethrough Mark = "strikethrough"
	MarkCode          Mark = "code"
)

// InlineType 是 v1 的行内节点类型判别值。
type InlineType string

const (
	InlineText              InlineType = "text"
	InlineCode              InlineType = "inline_code"
	InlinePageReference     InlineType = "page_reference"
	InlineExternalLink      InlineType = "external_link"
	InlineEntityReference   InlineType = "entity_reference"
	InlineClaimReference    InlineType = "claim_reference"
	InlineCitationReference InlineType = "citation_reference"
)

// ResolutionUnresolved 是未解析页面引用的 resolution_status 取值。
const ResolutionUnresolved = "unresolved"

// Document 是 AST v1 根节点。Type 恒为 "document"，SchemaVersion 恒为 1。
type Document struct {
	Type          string   `json:"type"`
	SchemaVersion int      `json:"schema_version"`
	Children      []*Block `json:"children"`
}

// NewDocument 创建空文档。
func NewDocument() *Document {
	return &Document{Type: "document", SchemaVersion: SchemaVersion, Children: []*Block{}}
}

// Block 是任意 Block 类型的通用载体：Go 无判别联合，
// 字段合法性由 Validate 依据 JSON Schema 裁决（容器规则、additionalProperties 等）。
//
// Content 依类型不同为 []InlineNode（heading/paragraph）或 string（code），
// 故保留原始 JSON，用 InlineContent / TextContent 解码。
type Block struct {
	ID       string      `json:"id"`
	Type     BlockType   `json:"type"`
	Level    int         `json:"level,omitempty"`
	Kind     CalloutKind `json:"kind,omitempty"`
	Language string      `json:"language,omitempty"`
	// ComponentBlock（M9-T03）：引用冻结组件版本和稳定 Entity，
	// display_config 只保存展示配置，不复制 Claim 值。
	ComponentID      string          `json:"component_id,omitempty"`
	ComponentVersion int             `json:"component_version,omitempty"`
	EntityID         string          `json:"entity_id,omitempty"`
	DisplayConfig    json.RawMessage `json:"display_config,omitempty"`
	Content          json.RawMessage `json:"content,omitempty"`
	Children         []*Block        `json:"children,omitempty"`
}

// InlineContent 解码 heading/paragraph 的 content 为行内节点数组。
func (b *Block) InlineContent() ([]*InlineNode, error) {
	if len(b.Content) == 0 {
		return nil, nil
	}
	var nodes []*InlineNode
	if err := json.Unmarshal(b.Content, &nodes); err != nil {
		return nil, fmt.Errorf("ast: 解码 Block %s 的行内 content 失败: %w", b.ID, err)
	}
	return nodes, nil
}

// TextContent 解码 code Block 的 content 为字符串。
func (b *Block) TextContent() (string, error) {
	if len(b.Content) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(b.Content, &s); err != nil {
		return "", fmt.Errorf("ast: 解码 Block %s 的文本 content 失败: %w", b.ID, err)
	}
	return s, nil
}

// InlineNode 是任意行内节点类型的通用载体，字段合法性由 Schema 校验裁决。
type InlineNode struct {
	Type        InlineType `json:"type"`
	Text        string     `json:"text,omitempty"`
	Marks       []Mark     `json:"marks,omitempty"`
	URL         string     `json:"url,omitempty"`
	DisplayText string     `json:"display_text,omitempty"`

	// 已解析页面引用。
	TargetPageID         string `json:"target_page_id,omitempty"`
	TargetHeadingBlockID string `json:"target_heading_block_id,omitempty"`

	// 未解析页面引用（resolution_status = "unresolved"）。
	ResolutionStatus   string `json:"resolution_status,omitempty"`
	TargetNamespace    string `json:"target_namespace,omitempty"`
	NormalizedTitle    string `json:"normalized_title,omitempty"`
	ExpectedEntityType string `json:"expected_entity_type,omitempty"`

	// 知识引用（M4-T06）：实体 / Claim / Citation 的稳定 ID。
	EntityID   string `json:"entity_id,omitempty"`
	ClaimID    string `json:"claim_id,omitempty"`
	CitationID string `json:"citation_id,omitempty"`
}

var compiledSchema = mustCompileSchema()

func mustCompileSchema() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		panic(fmt.Sprintf("ast: 解析内嵌 Schema 失败: %v", err))
	}
	c := jsonschema.NewCompiler()
	// uuid/uri 等 format 在 draft 2020-12 默认为注解，这里显式开启断言。
	c.AssertFormat()
	if err := c.AddResource(SchemaURL, doc); err != nil {
		panic(fmt.Sprintf("ast: 注册内嵌 Schema 失败: %v", err))
	}
	sch, err := c.Compile(SchemaURL)
	if err != nil {
		panic(fmt.Sprintf("ast: 编译内嵌 Schema 失败: %v", err))
	}
	return sch
}

// Validate 将 doc 序列化后按 AST v1 Schema 校验。
func Validate(doc *Document) error {
	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("ast: 序列化文档失败: %w", err)
	}
	return ValidateJSON(data)
}

// ValidateJSON 按 AST v1 Schema 校验原始 JSON 文档。
func ValidateJSON(data []byte) error {
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("ast: 解析 JSON 失败: %w", err)
	}
	if err := compiledSchema.Validate(inst); err != nil {
		return fmt.Errorf("ast: Schema 校验失败: %w", err)
	}
	return nil
}

// Parse 校验并解码 AST v1 文档。
func Parse(data []byte) (*Document, error) {
	if err := ValidateJSON(data); err != nil {
		return nil, err
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("ast: 解码文档失败: %w", err)
	}
	return &doc, nil
}

var defaultGenerator = id.NewGenerator()

// NewID 生成新的 Block ID（UUIDv7，ADR-0008）。
func NewID() (string, error) {
	s, err := defaultGenerator.NewString()
	if err != nil {
		return "", fmt.Errorf("ast: 生成 Block ID 失败: %w", err)
	}
	return s, nil
}
