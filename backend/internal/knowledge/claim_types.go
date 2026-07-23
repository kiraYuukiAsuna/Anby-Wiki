package knowledge

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

// Claim 领域的哨兵错误，调用方用 errors.Is 判定；具体上下文通过 %w 包装附加。
var (
	// ErrPropertyNotFound property_key 未命中 property（000004 种子或后续登记）。
	ErrPropertyNotFound = errors.New("knowledge: property 不存在")
	// ErrClaimNotFound 目标 Claim 不存在。
	ErrClaimNotFound = errors.New("knowledge: claim 不存在")
	// ErrInvalidClaimValue 值形态与 property.value_type 不符，或值本身非法
	//（空串、NaN/Inf、非 RFC3339 date、坐标越界、composite 非 object 或不满足 schema_json）。
	ErrInvalidClaimValue = errors.New("knowledge: claim 值非法")
	// ErrInvalidOriginType origin_type 不是 human/ai/import。
	ErrInvalidOriginType = errors.New("knowledge: origin_type 非法")
	// ErrInvalidRank rank 不是 preferred/normal/deprecated。
	ErrInvalidRank = errors.New("knowledge: rank 非法")
	// ErrInvalidValidTime valid_from/valid_to 两端都有但 valid_to <= valid_from
	//（DB 有 claim_valid_time_check 兜底，服务层先给领域错误）。
	ErrInvalidValidTime = errors.New("knowledge: valid_to 必须晚于 valid_from")
	// ErrSubjectTypeMismatch property 声明了 subject_type（列或 schema_json），
	// 但 subject 实体类型不匹配。
	ErrSubjectTypeMismatch = errors.New("knowledge: subject 实体类型与 property.subject_type 不匹配")
	// ErrTargetTypeMismatch entity 值的目标实体类型与 property.target_type 不匹配。
	ErrTargetTypeMismatch = errors.New("knowledge: target 实体类型与 property.target_type 不匹配")
	// ErrClaimNotMultivalued property.is_multivalued=false 且同 subject+property
	// 已有 published claim；提示调用方改用 SupersedeClaim。
	ErrClaimNotMultivalued = errors.New("knowledge: 单值 property 已存在 published claim，应先 Supersede")
	// ErrInvalidClaimTransition 业务状态机不允许的转换（含终态再流转、
	// 已 superseded 再 supersede、绕过 SupersedeClaim 直接置 superseded）。
	ErrInvalidClaimTransition = errors.New("knowledge: claim 状态转换非法")
	// ErrClaimSubjectMismatch SupersedeClaim 的 subject/property 断言与旧 claim 实际值不符。
	ErrClaimSubjectMismatch = errors.New("knowledge: supersede 断言与旧 claim 的 subject/property 不符")
	// ErrInvalidClaimStatus status 不是 proposed/published/rejected/superseded/deprecated
	//（ListClaims 过滤参数的防御性校验）。
	ErrInvalidClaimStatus = errors.New("knowledge: claim status 非法")
	// ErrInvalidVerificationStatus verification_status 不是 unverified/ai_checked/human_verified/disputed。
	ErrInvalidVerificationStatus = errors.New("knowledge: verification_status 非法")
	// ErrVerificationForbidden 该 Actor 类型无权设置目标验证状态
	//（human_verified/disputed 仅 human；ai 只能置 ai_checked；bot/system 不可改验证状态）。
	ErrVerificationForbidden = errors.New("knowledge: 无权设置该验证状态")
	// ErrInvalidSupportType support_type 不是 supports/contradicts/context。
	ErrInvalidSupportType = errors.New("knowledge: support_type 非法")
	// ErrClaimSourceExists 同 (claim, citation) 来源已存在（幂等拒绝）。
	ErrClaimSourceExists = errors.New("knowledge: claim 来源已存在")
	// ErrClaimTerminal claim 已 superseded/deprecated，拒绝新增来源。
	ErrClaimTerminal = errors.New("knowledge: claim 已终结，拒绝新增来源")
	// ErrCitationNotFound 绑定的 citation 不存在（INV-07 存在性校验，M4-T05 落地）。
	ErrCitationNotFound = errors.New("knowledge: citation 不存在")
)

// 值类型（property.value_type / claim.value_type，000003 CHECK 同款枚举）。
const (
	// ValueTypeString 字符串值，value_json 为 JSON string。
	ValueTypeString = "string"
	// ValueTypeNumber 数字值，value_json 为 JSON number。
	ValueTypeNumber = "number"
	// ValueTypeDate 日期值，value_json 为 RFC3339 date（YYYY-MM-DD）JSON string。
	ValueTypeDate = "date"
	// ValueTypeEntity 实体值，value_json 为 {"entity_id": "<uuid>"}，并冗余 target_entity_id 列。
	ValueTypeEntity = "entity"
	// ValueTypeCoordinate 坐标值，value_json 为 {"lat": <number>, "lon": <number>}。
	ValueTypeCoordinate = "coordinate"
	// ValueTypeComposite 复合值，value_json 为自由 JSON object；
	// property.schema_json.value 非空时按其子集 Schema 校验。
	ValueTypeComposite = "composite"
)

// Claim 投影失效事件。payload 只含稳定 ID，不包含 Claim 值或来源正文。
const (
	AggregateTypeClaim      = "claim"
	OutboxEventClaimChanged = "claim.changed"
)

// Claim 业务状态（claim.status，设计 §6.5）。与验证状态正交。
const (
	// ClaimStatusProposed 提议中（ai/import 来源的初始状态，待 M5 审核转 published）。
	ClaimStatusProposed = "proposed"
	// ClaimStatusPublished 已发布（human/bot 来源的初始状态）。
	ClaimStatusPublished = "published"
	// ClaimStatusRejected 已拒绝（终态）。
	ClaimStatusRejected = "rejected"
	// ClaimStatusSuperseded 已被新 claim 取代（终态，只能经 SupersedeClaim 链产生）。
	ClaimStatusSuperseded = "superseded"
	// ClaimStatusDeprecated 已废弃（终态）。
	ClaimStatusDeprecated = "deprecated"
)

// Claim 验证状态（claim.verification_status，设计 §6.5）。与业务状态正交。
const (
	// VerificationUnverified 未验证（初始状态）。
	VerificationUnverified = "unverified"
	// VerificationAIChecked AI 已核对。
	VerificationAIChecked = "ai_checked"
	// VerificationHumanVerified 人工已验证（INV-07）。
	VerificationHumanVerified = "human_verified"
	// VerificationDisputed 存在争议。
	VerificationDisputed = "disputed"
)

// Claim 来源类型（claim.origin_type，000003 CHECK）。
const (
	// OriginHuman 人工录入。
	OriginHuman = "human"
	// OriginAI AI 抽取（初始 proposed，INV-08 治理前置，M5 审核后转 published）。
	OriginAI = "ai"
	// OriginImport 批量导入（初始 proposed）。
	OriginImport = "import"
)

// Claim 优先级（claim.rank，000003 CHECK）。
const (
	// RankPreferred 同组多值中的首选值。
	RankPreferred = "preferred"
	// RankNormal 默认优先级。
	RankNormal = "normal"
	// RankDeprecated 不再推荐展示的历史值。
	RankDeprecated = "deprecated"
)

// claim_source.support_type（000003 CHECK）。
const (
	// SupportTypeSupports 来源支持该 claim。
	SupportTypeSupports = "supports"
	// SupportTypeContradicts 来源反驳该 claim（冲突值证据，设计 §6.4）。
	SupportTypeContradicts = "contradicts"
	// SupportTypeContext 来源仅提供上下文。
	SupportTypeContext = "context"
)

// Property Claim 谓词定义（对应 property 表，000004 固定 UUID 种子）。
type Property struct {
	ID            uuid.UUID
	PropertyKey   string
	Name          string
	ValueType     string
	SubjectTypeID *uuid.UUID
	TargetTypeID  *uuid.UUID
	IsMultivalued bool
	SchemaJSON    json.RawMessage
	CreatedAt     time.Time
}

// Claim 结构化事实（对应 claim 表）。业务状态（Status）与验证状态
// （VerificationStatus）两个维度正交（设计 §6.5）。
type Claim struct {
	ID                 uuid.UUID
	SubjectEntityID    uuid.UUID
	PropertyID         uuid.UUID
	ValueType          string
	ValueJSON          json.RawMessage
	TargetEntityID     *uuid.UUID
	QualifiersJSON     json.RawMessage
	Rank               string
	Status             string
	VerificationStatus string
	ValidFrom          *time.Time
	ValidTo            *time.Time
	OriginType         string
	ChangeBatchID      *uuid.UUID
	CreatedBy          uuid.UUID
	CreatedAt          time.Time
	SupersededBy       *uuid.UUID
}

// ClaimSource Claim 与 Citation 的关联（对应 claim_source 表，PK = claim+citation）。
type ClaimSource struct {
	ClaimID     uuid.UUID
	CitationID  uuid.UUID
	SupportType string
	CreatedAt   time.Time
}

// Coordinate 坐标值（lat ∈ [-90,90]，lon ∈ [-180,180]）。
type Coordinate struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Value Claim 值入参：按 property.value_type 取对应字段，其余字段忽略。
// 构造用 StringValue/NumberValue/DateValue/EntityValue/CoordinateValue/CompositeValue。
type Value struct {
	String     *string
	Number     *float64
	Date       *string
	EntityID   *uuid.UUID
	Coordinate *Coordinate
	Composite  json.RawMessage
}

// StringValue 构造字符串值。
func StringValue(s string) Value { return Value{String: &s} }

// NumberValue 构造数字值。
func NumberValue(f float64) Value { return Value{Number: &f} }

// DateValue 构造日期值（RFC3339 date，YYYY-MM-DD）。
func DateValue(s string) Value { return Value{Date: &s} }

// EntityValue 构造实体值。
func EntityValue(id uuid.UUID) Value { return Value{EntityID: &id} }

// CoordinateValue 构造坐标值。
func CoordinateValue(lat, lon float64) Value {
	return Value{Coordinate: &Coordinate{Lat: lat, Lon: lon}}
}

// CompositeValue 构造复合值（自由 JSON object）。
func CompositeValue(raw json.RawMessage) Value { return Value{Composite: raw} }

// propertySchema property.schema_json 的服务层约定子集（000004 种子注释：
// 类型约束由服务层按 schema_json 校验）。全部字段可选；空 object 表示无附加约束。
// TODO(M5): 这是简版实现——composite 只支持 required + 基本类型、subject/target
// 只支持单一 type_key；完整 JSON Schema 校验与多类型约束由 M5 治理迭代时完善。
type propertySchema struct {
	// SubjectType subject 实体必须属于的 entity_type.type_key（与 SubjectTypeID 列并列校验）。
	SubjectType string `json:"subject_type"`
	// TargetType entity 值目标必须属于的 entity_type.type_key（与 TargetTypeID 列并列校验）。
	TargetType string `json:"target_type"`
	// Value composite 值的最小 JSON-Schema 子集：required 必填键 +
	// properties.<key>.type 基本类型（string/number/boolean/object/array/null）。
	Value *compositeSchema `json:"value"`
}

type compositeSchema struct {
	Required   []string                    `json:"required"`
	Properties map[string]compositeTypeDef `json:"properties"`
}

type compositeTypeDef struct {
	Type string `json:"type"`
}

// parsePropertySchema 解析 property.schema_json；空/{} 返回零值 schema（无附加约束）。
func parsePropertySchema(raw json.RawMessage) (*propertySchema, error) {
	var s propertySchema
	if len(raw) == 0 {
		return &s, nil
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("%w: property.schema_json 解析失败: %v", ErrInvalidClaimValue, err)
	}
	return &s, nil
}

// normalizeValue 按 property.value_type 校验并规范化值，产出落库的 value_json
// 与 target_entity_id（仅 entity 值非 nil）。property.schema_json 的非空约束
// （composite 子集 Schema）在此一并校验；类型形态之外不做额外业务校验。
func normalizeValue(prop *Property, v Value) (valueJSON json.RawMessage, targetEntityID *uuid.UUID, err error) {
	switch prop.ValueType {
	case ValueTypeString:
		if v.String == nil || *v.String == "" {
			return nil, nil, fmt.Errorf("%w: property=%q string 值不能为空", ErrInvalidClaimValue, prop.PropertyKey)
		}
		valueJSON, err = json.Marshal(*v.String)
	case ValueTypeNumber:
		if v.Number == nil || math.IsNaN(*v.Number) || math.IsInf(*v.Number, 0) {
			return nil, nil, fmt.Errorf("%w: property=%q number 值缺失或为 NaN/Inf", ErrInvalidClaimValue, prop.PropertyKey)
		}
		valueJSON, err = json.Marshal(*v.Number)
	case ValueTypeDate:
		if v.Date == nil {
			return nil, nil, fmt.Errorf("%w: property=%q date 值缺失", ErrInvalidClaimValue, prop.PropertyKey)
		}
		if _, perr := time.Parse("2006-01-02", *v.Date); perr != nil {
			return nil, nil, fmt.Errorf("%w: property=%q date 值 %q 不是 RFC3339 date", ErrInvalidClaimValue, prop.PropertyKey, *v.Date)
		}
		valueJSON, err = json.Marshal(*v.Date)
	case ValueTypeEntity:
		if v.EntityID == nil || *v.EntityID == uuid.Nil {
			return nil, nil, fmt.Errorf("%w: property=%q entity 值必须提供 entity_id", ErrInvalidClaimValue, prop.PropertyKey)
		}
		id := *v.EntityID
		valueJSON, err = json.Marshal(map[string]uuid.UUID{"entity_id": id})
		targetEntityID = &id
	case ValueTypeCoordinate:
		if v.Coordinate == nil {
			return nil, nil, fmt.Errorf("%w: property=%q coordinate 值缺失", ErrInvalidClaimValue, prop.PropertyKey)
		}
		c := v.Coordinate
		if math.IsNaN(c.Lat) || math.IsNaN(c.Lon) || math.IsInf(c.Lat, 0) || math.IsInf(c.Lon, 0) ||
			c.Lat < -90 || c.Lat > 90 || c.Lon < -180 || c.Lon > 180 {
			return nil, nil, fmt.Errorf("%w: property=%q 坐标越界 lat=%v lon=%v", ErrInvalidClaimValue, prop.PropertyKey, c.Lat, c.Lon)
		}
		valueJSON, err = json.Marshal(c)
	case ValueTypeComposite:
		if len(v.Composite) == 0 {
			return nil, nil, fmt.Errorf("%w: property=%q composite 值缺失", ErrInvalidClaimValue, prop.PropertyKey)
		}
		var obj map[string]json.RawMessage
		if uerr := json.Unmarshal(v.Composite, &obj); uerr != nil || obj == nil {
			return nil, nil, fmt.Errorf("%w: property=%q composite 值必须是 JSON object", ErrInvalidClaimValue, prop.PropertyKey)
		}
		schema, serr := parsePropertySchema(prop.SchemaJSON)
		if serr != nil {
			return nil, nil, serr
		}
		if schema.Value != nil {
			if verr := validateComposite(schema.Value, obj); verr != nil {
				return nil, nil, fmt.Errorf("%w: property=%q %v", ErrInvalidClaimValue, prop.PropertyKey, verr)
			}
		}
		valueJSON = v.Composite
	default:
		return nil, nil, fmt.Errorf("%w: property=%q 未知 value_type %q", ErrInvalidClaimValue, prop.PropertyKey, prop.ValueType)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("knowledge: 编码 claim 值失败: %w", err)
	}
	return valueJSON, targetEntityID, nil
}

// validateComposite 按 compositeSchema 子集校验 composite 值对象。
func validateComposite(schema *compositeSchema, obj map[string]json.RawMessage) error {
	for _, key := range schema.Required {
		if _, ok := obj[key]; !ok {
			return fmt.Errorf("composite 缺少必填键 %q", key)
		}
	}
	for key, def := range schema.Properties {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		if !jsonTypeMatches(raw, def.Type) {
			return fmt.Errorf("composite 键 %q 类型应为 %q", key, def.Type)
		}
	}
	return nil
}

// jsonTypeMatches 判断 JSON 值的基本类型（string/number/boolean/object/array/null）。
func jsonTypeMatches(raw json.RawMessage, typ string) bool {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	switch typ {
	case "string":
		_, ok := v.(string)
		return ok
	case "number":
		_, ok := v.(float64)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "null":
		return v == nil
	default:
		return false
	}
}

// claimStatusTransitions 业务状态机（设计 §6.5）：
// proposed→published/rejected；published→deprecated/superseded；
// rejected/deprecated/superseded 为终态（superseded 只允许经 SupersedeClaim 链产生，
// 公开的状态流转方法从不以 superseded 为目标）。
var claimStatusTransitions = map[string][]string{
	ClaimStatusProposed:  {ClaimStatusPublished, ClaimStatusRejected},
	ClaimStatusPublished: {ClaimStatusDeprecated, ClaimStatusSuperseded},
}

// canTransitionClaim 判断业务状态 from→to 是否合法。
func canTransitionClaim(from, to string) bool {
	for _, next := range claimStatusTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// checkVerificationPermission 验证状态权限矩阵（纯函数，便于单测）：
// human 可置全部四种状态；ai 只能置 ai_checked；bot/system 无权修改验证状态。
func checkVerificationPermission(actorType, status string) error {
	switch status {
	case VerificationUnverified, VerificationAIChecked, VerificationHumanVerified, VerificationDisputed:
	default:
		return fmt.Errorf("%w: status=%q", ErrInvalidVerificationStatus, status)
	}
	switch actorType {
	case "human":
		return nil
	case "ai":
		if status == VerificationAIChecked {
			return nil
		}
		return fmt.Errorf("%w: ai actor 只能置 ai_checked，拒绝 %q", ErrVerificationForbidden, status)
	default:
		return fmt.Errorf("%w: actor_type=%q 无权修改验证状态", ErrVerificationForbidden, actorType)
	}
}

// isValidSupportType 校验 support_type 枚举。
func isValidSupportType(s string) bool {
	switch s {
	case SupportTypeSupports, SupportTypeContradicts, SupportTypeContext:
		return true
	}
	return false
}
