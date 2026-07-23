// Package knowledge 实现 Knowledge 领域服务：
//   - Entity（M4-T02）：Entity 创建、多语言标签/别名、规范化搜索与 Page 绑定。
//     Entity 是稳定身份，同名候选不自动合并（MT-M4-NO-AUTOMERGE）；
//     合并功能本身属 M9-T06，本模块只保证 merged 实体拒绝写入、搜索默认排除。
//   - Claim（M4-T03，claim.go/claim_types.go/claim_repository.go）：
//     Property/Claim/ClaimSource 生命周期——值类型校验、业务/验证双状态、Supersede 链。
package knowledge

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// 领域错误：哨兵 error，调用方用 errors.Is 判定；具体上下文通过 %w 包装附加。
var (
	// ErrEntityNotFound 实体不存在。
	ErrEntityNotFound = errors.New("knowledge: 实体不存在")
	// ErrEntityTypeNotFound 实体类型未种子（type_key 未命中 entity_type）。
	ErrEntityTypeNotFound = errors.New("knowledge: 实体类型不存在")
	// ErrDuplicateEntityKey 同 wiki 内 canonical_key 已被其他实体占用。
	ErrDuplicateEntityKey = errors.New("knowledge: canonical_key 冲突")
	// ErrInvalidLabel 标签为空、超过 255 字符或含控制字符（规则同 page 标题）。
	ErrInvalidLabel = errors.New("knowledge: 标签非法")
	// ErrNoPrimaryLabel 创建时未提供主标签，或删除会导致实体不再有任何主标签。
	ErrNoPrimaryLabel = errors.New("knowledge: 至少需要一个主标签")
	// ErrDuplicatePrimaryLabel 同 entity + language 已存在其他主标签。
	ErrDuplicatePrimaryLabel = errors.New("knowledge: 同语言主标签已存在")
	// ErrLabelExists 同 (entity, language, label) 标签已存在。
	ErrLabelExists = errors.New("knowledge: 标签已存在")
	// ErrLabelNotFound 目标标签不存在。
	ErrLabelNotFound = errors.New("knowledge: 标签不存在")
	// ErrDuplicateAlias 同实体内规范化别名重复。
	ErrDuplicateAlias = errors.New("knowledge: 别名重复")
	// ErrAliasNotFound 目标别名不存在。
	ErrAliasNotFound = errors.New("knowledge: 别名不存在")
	// ErrEntityMerged 实体已合并，拒绝写入（标签/别名/绑定）；合并见 M9-T06。
	ErrEntityMerged = errors.New("knowledge: 实体已合并")
	// ErrInvalidEntityMerge 合并源/目标不满足同站点、同类型、active 等约束。
	ErrInvalidEntityMerge = errors.New("knowledge: Entity 合并非法")
	// ErrEntityMergeNotFound 合并批次不存在。
	ErrEntityMergeNotFound = errors.New("knowledge: Entity 合并记录不存在")
	// ErrEntityMergeCycle merged_into 链存在环或无法收敛到 active Entity。
	ErrEntityMergeCycle = errors.New("knowledge: Entity merged 映射存在环")
	// ErrEntityMergeStale 合并后的标签、Claim 或 merged 指针已变化，拒绝静默补偿。
	ErrEntityMergeStale = errors.New("knowledge: Entity 合并后已有新变更")
	// ErrEntityMergeActorOnly Entity 合并与补偿仅允许 human/system。
	ErrEntityMergeActorOnly = errors.New("knowledge: Entity 合并仅允许 human/system")
	// ErrInvalidBindingRole 绑定角色不是 primary/mentioned。
	ErrInvalidBindingRole = errors.New("knowledge: 绑定角色非法")
	// ErrBindingExists 同 (page, entity, role) 绑定已存在。
	ErrBindingExists = errors.New("knowledge: 绑定已存在")
	// ErrBindingNotFound 目标绑定不存在。
	ErrBindingNotFound = errors.New("knowledge: 绑定不存在")
)

// 实体状态（entity.status）。
const (
	// StatusActive 正常实体。
	StatusActive = "active"
	// StatusMerged 已合并进其他实体（merged_into_entity_id 指向目标）。
	StatusMerged = "merged"
	// StatusDeleted 软删除。
	StatusDeleted = "deleted"
)

// 绑定角色（page_entity_binding.binding_role）。
const (
	// BindingRolePrimary 页面的主实体。
	BindingRolePrimary = "primary"
	// BindingRoleMentioned 页面提及的实体。
	BindingRoleMentioned = "mentioned"
)

// 别名类型（entity_alias.alias_type，设计 §6.2）。
const (
	// AliasTypeCommon 通用别名（默认）。
	AliasTypeCommon = "common"
	// AliasTypeHistorical 历史名称。
	AliasTypeHistorical = "historical"
	// AliasTypeAbbreviation 缩写。
	AliasTypeAbbreviation = "abbreviation"
	// AliasTypeImport 导入名称。
	AliasTypeImport = "import"
)

// 搜索命中位置（SearchResult.MatchedOn）。
const (
	// MatchedOnCanonical 命中 canonical_key。
	MatchedOnCanonical = "canonical"
	// MatchedOnLabel 命中标签。
	MatchedOnLabel = "label"
	// MatchedOnAlias 命中别名。
	MatchedOnAlias = "alias"
)

// EntityType 实体类型（对应 entity_type 表，000004 固定 UUID 种子）。
type EntityType struct {
	ID        uuid.UUID
	TypeKey   string
	Name      string
	CreatedAt time.Time
}

// Entity 实体身份（对应 entity 表）。
type Entity struct {
	ID                 uuid.UUID
	WikiID             uuid.UUID
	EntityTypeID       uuid.UUID
	CanonicalKey       string
	Status             string
	MergedIntoEntityID *uuid.UUID
	CreatedBy          uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// EntityLabel 实体多语言标签（对应 entity_label 表，PK = entity+language+label）。
type EntityLabel struct {
	EntityID    uuid.UUID
	Language    string
	Label       string
	Description string
	IsPrimary   bool
}

// EntityAlias 实体别名（对应 entity_alias 表）。
type EntityAlias struct {
	ID              uuid.UUID
	EntityID        uuid.UUID
	Language        string
	Alias           string
	NormalizedAlias string
	AliasType       string
	CreatedAt       time.Time
}

// PageEntityBinding 页面与实体的绑定（对应 page_entity_binding 表）。
type PageEntityBinding struct {
	PageID    uuid.UUID
	EntityID  uuid.UUID
	Role      string
	Language  string
	CreatedAt time.Time
}

// SearchResult 单条搜索结果：实体 + 命中位置 + 是否精确命中。
type SearchResult struct {
	Entity    *Entity
	MatchedOn string
	Exact     bool
}
