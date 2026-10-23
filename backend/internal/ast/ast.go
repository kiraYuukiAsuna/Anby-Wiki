// Package ast 提供 Typed Block AST v1 的 Go 类型与 Schema 校验。
//
// 权威契约是 contracts/schemas/ast/v1/ast.schema.json（JSON Schema draft 2020-12），
// 本包内嵌其字节级副本（schema/ast.schema.json）用于运行时校验；
// 副本与原文件的一致性由单测与 scripts/check-contracts.sh 保证。
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
	BlockHeading      BlockType = "heading"
	BlockParagraph    BlockType = "paragraph"
	BlockBulletList   BlockType = "bullet_list"
	BlockOrderedList  BlockType = "ordered_list"
	BlockListItem     BlockType = "list_item"
	BlockTable        BlockType = "table"
	BlockTableRow     BlockType = "table_row"
	BlockTableCell    BlockType = "table_cell"
	BlockCode         BlockType = "code"
	BlockPreformatted BlockType = "preformatted"
	BlockEquation     BlockType = "equation"
	BlockTodo         BlockType = "todo"
	BlockToggle       BlockType = "toggle"
	BlockBreadcrumb   BlockType = "breadcrumb"
	BlockColumnList   BlockType = "column_list"
	BlockColumn       BlockType = "column"
	BlockButton       BlockType = "button"
	BlockHTML         BlockType = "html"
	BlockQuote        BlockType = "quote"
	BlockCallout      BlockType = "callout"
	BlockComponent    BlockType = "component"
	BlockImage        BlockType = "image"
	BlockVideo        BlockType = "video"
	BlockAudio        BlockType = "audio"
	BlockFile         BlockType = "file"
	BlockPDF          BlockType = "pdf"
	BlockBookmark     BlockType = "bookmark"
	BlockLinkPreview  BlockType = "link_preview"
	BlockDatasetView  BlockType = "dataset_view"
	BlockEmbed        BlockType = "embed"
	BlockDivider      BlockType = "divider"
)

// CalloutKind 是 callout Block 的种类。
type CalloutKind string

const (
	CalloutInfo    CalloutKind = "info"
	CalloutWarning CalloutKind = "warning"
	CalloutDanger  CalloutKind = "danger"
)

// CalloutColor 是 callout 的可选展示色；空值保持 kind 对应的默认语义色。
type CalloutColor string

const (
	CalloutColorDefault CalloutColor = "default"
	CalloutColorGray    CalloutColor = "gray"
	CalloutColorBrown   CalloutColor = "brown"
	CalloutColorOrange  CalloutColor = "orange"
	CalloutColorYellow  CalloutColor = "yellow"
	CalloutColorGreen   CalloutColor = "green"
	CalloutColorBlue    CalloutColor = "blue"
	CalloutColorPurple  CalloutColor = "purple"
	CalloutColorPink    CalloutColor = "pink"
	CalloutColorRed     CalloutColor = "red"
)

// PreformattedKind 区分固定宽度原文的内容语义，避免把图表误称为代码。
type PreformattedKind string

const (
	PreformattedASCIIDiagram  PreformattedKind = "ascii_diagram"
	PreformattedFixedTable    PreformattedKind = "fixed_table"
	PreformattedProtocolTrace PreformattedKind = "protocol_trace"
	PreformattedFormulaText   PreformattedKind = "formula_text"
	PreformattedPlain         PreformattedKind = "plain"
)

// ButtonActionType 是页面按钮按顺序执行的声明式动作类型。
type ButtonActionType string

const (
	ButtonActionInsertBlocks        ButtonActionType = "insert_blocks"
	ButtonActionUpdateDatasetRecord ButtonActionType = "update_dataset_record"
	ButtonActionNotification        ButtonActionType = "notification"
	ButtonActionWebhook             ButtonActionType = "webhook"
)

// Mark 是 text 行内节点的修饰。
type Mark string

const (
	MarkBold          Mark = "bold"
	MarkItalic        Mark = "italic"
	MarkUnderline     Mark = "underline"
	MarkStrikethrough Mark = "strikethrough"
	MarkCode          Mark = "code"
)

// InlineColor 是行内文本和背景使用的受限展示色。
type InlineColor string

const (
	InlineColorGray   InlineColor = "gray"
	InlineColorBrown  InlineColor = "brown"
	InlineColorRed    InlineColor = "red"
	InlineColorOrange InlineColor = "orange"
	InlineColorYellow InlineColor = "yellow"
	InlineColorGreen  InlineColor = "green"
	InlineColorBlue   InlineColor = "blue"
	InlineColorPurple InlineColor = "purple"
	InlineColorPink   InlineColor = "pink"
)

// InlineType 是 v1 的行内节点类型判别值。
type InlineType string

const (
	InlineText                InlineType = "text"
	InlineCode                InlineType = "inline_code"
	InlinePageReference       InlineType = "page_reference"
	InlinePageAnchorReference InlineType = "page_anchor_reference"
	InlineExternalLink        InlineType = "external_link"
	InlineEntityReference     InlineType = "entity_reference"
	InlineClaimReference      InlineType = "claim_reference"
	InlineCitationReference   InlineType = "citation_reference"
	InlineMath                InlineType = "math"
	InlineMention             InlineType = "mention"
	InlineDateMention         InlineType = "date_mention"
	InlineDatabaseMention     InlineType = "database_mention"
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

// BreadcrumbItem 是 breadcrumb 中的一段页面路径；当前页可不含 TargetPageID。
type BreadcrumbItem struct {
	TargetPageID string `json:"target_page_id,omitempty"`
	DisplayText  string `json:"display_text"`
}

// ButtonAction 保存经 Schema 判别的页面按钮动作。各动作只使用对应字段；
// 权威写入由交互层调用领域服务，渲染器不会直接执行副作用。
type ButtonAction struct {
	Type         ButtonActionType `json:"type"`
	TargetPageID string           `json:"target_page_id,omitempty"`
	Blocks       []*Block         `json:"blocks,omitempty"`
	RecordID     string           `json:"record_id,omitempty"`
	EntityID     *string          `json:"entity_id,omitempty"`
	Values       json.RawMessage  `json:"values,omitempty"`
	Message      string           `json:"message,omitempty"`
	Tone         string           `json:"tone,omitempty"`
	URL          string           `json:"url,omitempty"`
	Payload      json.RawMessage  `json:"payload,omitempty"`
}

// Block 是任意 Block 类型的通用载体：Go 无判别联合，
// 字段合法性由 Validate 依据 JSON Schema 裁决（容器规则、additionalProperties 等）。
//
// Content 依类型不同为 []InlineNode（heading/paragraph/todo/toggle）或
// string（code/preformatted），
// 故保留原始 JSON，用 InlineContent / TextContent 解码。
type Block struct {
	ID              string           `json:"id"`
	Type            BlockType        `json:"type"`
	Level           int              `json:"level,omitempty"`
	IsToggleable    bool             `json:"is_toggleable,omitempty"`
	Checked         *bool            `json:"checked,omitempty"`
	Kind            string           `json:"kind,omitempty"`
	Icon            string           `json:"icon,omitempty"`
	Color           CalloutColor     `json:"color,omitempty"`
	Language        string           `json:"language,omitempty"`
	Expression      string           `json:"expression,omitempty"`
	HasColumnHeader bool             `json:"has_column_header,omitempty"`
	HasRowHeader    bool             `json:"has_row_header,omitempty"`
	Items           []BreadcrumbItem `json:"items,omitempty"`
	WidthRatio      float64          `json:"width_ratio,omitempty"`
	Label           string           `json:"label,omitempty"`
	Confirmation    string           `json:"confirmation,omitempty"`
	Actions         []ButtonAction   `json:"actions,omitempty"`
	HTML            string           `json:"html,omitempty"`
	Height          int              `json:"height,omitempty"`
	Name            string           `json:"name,omitempty"`
	Artist          string           `json:"artist,omitempty"`
	Album           string           `json:"album,omitempty"`
	DurationSeconds *float64         `json:"duration_seconds,omitempty"`
	StartPage       int              `json:"start_page,omitempty"`
	Description     string           `json:"description,omitempty"`
	SiteName        string           `json:"site_name,omitempty"`
	Provider        string           `json:"provider,omitempty"`
	// ComponentBlock（M9-T03）：引用冻结组件版本和稳定 Entity，
	// display_config 只保存展示配置，不复制 Claim 值。
	ComponentID      string          `json:"component_id,omitempty"`
	ComponentVersion int             `json:"component_version,omitempty"`
	EntityID         string          `json:"entity_id,omitempty"`
	DisplayConfig    json.RawMessage `json:"display_config,omitempty"`
	// 媒体、DatasetView 与安全外部嵌入。
	AssetRevisionID       string          `json:"asset_revision_id,omitempty"`
	PosterAssetRevisionID string          `json:"poster_asset_revision_id,omitempty"`
	AltText               string          `json:"alt_text,omitempty"`
	Caption               string          `json:"caption,omitempty"`
	Title                 string          `json:"title,omitempty"`
	DatasetViewID         string          `json:"dataset_view_id,omitempty"`
	URL                   string          `json:"url,omitempty"`
	Content               json.RawMessage `json:"content,omitempty"`
	Children              []*Block        `json:"children,omitempty"`
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
	Type            InlineType  `json:"type"`
	Text            string      `json:"text,omitempty"`
	Marks           []Mark      `json:"marks,omitempty"`
	TextColor       InlineColor `json:"text_color,omitempty"`
	BackgroundColor InlineColor `json:"background_color,omitempty"`
	URL             string      `json:"url,omitempty"`
	DisplayText     string      `json:"display_text,omitempty"`

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

	// Math 与 Mention。
	Expression string `json:"expression,omitempty"`
	ActorID    string `json:"actor_id,omitempty"`
	Date       string `json:"date,omitempty"`
	EndDate    string `json:"end_date,omitempty"`
	DatasetID  string `json:"dataset_id,omitempty"`
}

const documentEnvelopeSchemaURL = "https://anby.wiki/schemas/ast/v1/document-envelope.schema.json"

var blockSchemaDefinitions = map[BlockType]string{
	BlockHeading:      "headingBlock",
	BlockParagraph:    "paragraphBlock",
	BlockBulletList:   "bulletListBlock",
	BlockOrderedList:  "orderedListBlock",
	BlockListItem:     "listItemBlock",
	BlockTable:        "tableBlock",
	BlockTableRow:     "tableRowBlock",
	BlockTableCell:    "tableCellBlock",
	BlockCode:         "codeBlock",
	BlockPreformatted: "preformattedBlock",
	BlockEquation:     "equationBlock",
	BlockTodo:         "todoBlock",
	BlockToggle:       "toggleBlock",
	BlockBreadcrumb:   "breadcrumbBlock",
	BlockColumnList:   "columnListBlock",
	BlockColumn:       "columnBlock",
	BlockButton:       "buttonBlock",
	BlockHTML:         "htmlBlock",
	BlockQuote:        "quoteBlock",
	BlockCallout:      "calloutBlock",
	BlockComponent:    "componentBlock",
	BlockImage:        "imageBlock",
	BlockVideo:        "videoBlock",
	BlockAudio:        "audioBlock",
	BlockFile:         "fileBlock",
	BlockPDF:          "pdfBlock",
	BlockBookmark:     "bookmarkBlock",
	BlockLinkPreview:  "linkPreviewBlock",
	BlockDatasetView:  "datasetViewBlock",
	BlockEmbed:        "embedBlock",
	BlockDivider:      "dividerBlock",
}

var compiledDocumentEnvelope, compiledBlockSchemas = mustCompileSchemas()

func mustCompileSchemas() (*jsonschema.Schema, map[BlockType]*jsonschema.Schema) {
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
	blockSchemas := make(map[BlockType]*jsonschema.Schema, len(blockSchemaDefinitions))
	for blockType, definition := range blockSchemaDefinitions {
		schema, err := c.Compile(SchemaURL + "#/$defs/" + definition)
		if err != nil {
			panic(fmt.Sprintf("ast: 编译 %s Schema 失败: %v", blockType, err))
		}
		blockSchemas[blockType] = schema
	}

	envelope, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		panic(fmt.Sprintf("ast: 解析文档外壳 Schema 失败: %v", err))
	}
	envelopeObject := envelope.(map[string]any)
	envelopeObject["$id"] = documentEnvelopeSchemaURL
	properties := envelopeObject["properties"].(map[string]any)
	children := properties["children"].(map[string]any)
	children["items"] = true
	envelopeCompiler := jsonschema.NewCompiler()
	envelopeCompiler.AssertFormat()
	if err := envelopeCompiler.AddResource(documentEnvelopeSchemaURL, envelope); err != nil {
		panic(fmt.Sprintf("ast: 注册文档外壳 Schema 失败: %v", err))
	}
	documentEnvelope, err := envelopeCompiler.Compile(documentEnvelopeSchemaURL)
	if err != nil {
		panic(fmt.Sprintf("ast: 编译文档外壳 Schema 失败: %v", err))
	}
	return documentEnvelope, blockSchemas
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
	if err := compiledDocumentEnvelope.Validate(inst); err != nil {
		return fmt.Errorf("ast: Schema 校验失败: %w", err)
	}
	children := inst.(map[string]any)["children"].([]any)
	for _, child := range children {
		if err := validateBlockInstance(child); err != nil {
			return err
		}
	}
	return nil
}

// ValidateBlock 按 Block 的判别类型校验对应的权威 Schema 子定义。
func ValidateBlock(block *Block) error {
	if block == nil {
		return fmt.Errorf("ast: Block 不能为空")
	}
	data, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("ast: 序列化 Block 失败: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("ast: 解析 Block JSON 失败: %w", err)
	}
	return validateBlockInstance(inst)
}

func validateBlockInstance(inst any) error {
	object, ok := inst.(map[string]any)
	if !ok {
		return fmt.Errorf("ast: Schema 校验失败: Block 必须是 object")
	}
	rawType, ok := object["type"].(string)
	if !ok {
		return fmt.Errorf("ast: Schema 校验失败: Block 缺少有效 type")
	}
	schema, ok := compiledBlockSchemas[BlockType(rawType)]
	if !ok {
		return fmt.Errorf("ast: Schema 校验失败: 未知 Block type %q", rawType)
	}
	if err := schema.Validate(inst); err != nil {
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
