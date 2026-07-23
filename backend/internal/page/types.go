// Package page 实现 Page 身份与 Revision 发布的领域服务（M1-T04/T05）：
// 标题规范化、创建、查询、改名、别名解析、重定向与循环检测，
// 以及 Revision 发布事务（ContentSnapshot/Revision/current 指针/审计/Outbox 原子写入）。
// Page 是稳定身份，改名后 Page ID 不变；Revision 与 ContentSnapshot 发布后不可变。
package page

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// 领域错误：哨兵 error，调用方用 errors.Is 判定；具体上下文通过 %w 包装附加。
var (
	// ErrPageNotFound 页面不存在（含已软删除页面经标题解析的场景）。
	ErrPageNotFound = errors.New("page: 页面不存在")
	// ErrTitleConflict 规范化标题已被同 wiki+namespace 的活页面或别名占用。
	ErrTitleConflict = errors.New("page: 标题冲突")
	// ErrInvalidTitle 标题为空、超过 255 字符或含控制字符。
	ErrInvalidTitle = errors.New("page: 标题非法")
	// ErrInvalidActor Actor 不存在或已停用。
	ErrInvalidActor = errors.New("page: 无效 Actor")
	// ErrActorNotAllowed Actor 类型无权执行该操作（ai 直接创建/改名在 M5 治理前不允许）。
	ErrActorNotAllowed = errors.New("page: Actor 类型无权执行此操作")
	// ErrNamespaceNotFound 命名空间不存在。
	ErrNamespaceNotFound = errors.New("page: 命名空间不存在")
	// ErrRedirectLoop 重定向链存在环。
	ErrRedirectLoop = errors.New("page: 重定向循环")
	// ErrRedirectTooDeep 重定向链超过最大跳数。
	ErrRedirectTooDeep = errors.New("page: 重定向链过深")
	// ErrRedirectTargetDeleted 重定向目标页面已软删除。
	ErrRedirectTargetDeleted = errors.New("page: 重定向目标已删除")
	// ErrStaleRevision 发布的基线 Revision 与页面当前 Revision 不一致（含首发布断言失败）。
	ErrStaleRevision = errors.New("page: 基线 Revision 已过期")
	// ErrInvalidAST AST 未通过 v1 Schema 校验或 schema_version 非 1。
	ErrInvalidAST = errors.New("page: AST 校验失败")
	// ErrRevisionNotFound Revision 不存在或不属于指定页面（跨页访问不泄露存在性）。
	ErrRevisionNotFound = errors.New("page: Revision 不存在")
	// ErrInvalidCursor 历史列表的分页游标无法解析。
	ErrInvalidCursor = errors.New("page: 分页游标非法")
	// ErrWikiNotFound 站点不存在（如默认站点 site_key='default' 未种子）。
	ErrWikiNotFound = errors.New("page: 站点不存在")

	// errAliasNotFound 别名未命中（repository 内部使用，service 捕获后转领域语义）。
	errAliasNotFound = errors.New("page: 别名不存在")
	// errActorNotFound Actor 不存在（repository 内部使用）。
	errActorNotFound = errors.New("page: Actor 不存在")
)

// 页面状态（page.status）。
const (
	StatusActive = "active"
)

// 别名类型（page_alias.alias_type）。
const (
	// AliasTypeRename 改名产生的旧标题别名。
	AliasTypeRename = "rename"
)

// 允许创建/改名页面的 Actor 类型。
// ai 直接创建页面在 M5 治理落地前不允许；anonymous/import 同样不直接写页面。
var allowedWriteActorTypes = map[string]bool{
	"human":  true,
	"bot":    true,
	"system": true,
}

// Page 页面身份（对应 page 表）。
type Page struct {
	ID                uuid.UUID
	WikiID            uuid.UUID
	NamespaceID       uuid.UUID
	NormalizedTitle   string
	DisplayTitle      string
	Language          string
	ContentModel      string
	Status            string
	CurrentRevisionID *uuid.UUID
	PrimaryEntityID   *uuid.UUID
	CreatedBy         uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
}

// Alias 页面别名（对应 page_alias 表）。
type Alias struct {
	ID              uuid.UUID
	WikiID          uuid.UUID
	NamespaceID     uuid.UUID
	NormalizedTitle string
	PageID          uuid.UUID
	AliasType       string
	CreatedAt       time.Time
}

// Actor 行为主体（对应 actor 表，仅本模块校验所需字段）。
type Actor struct {
	ID          uuid.UUID
	ActorType   string
	DisplayName string
	Status      string
}

// ResolvedPage ResolveTitle 的结果：命中页面及是否经由别名解析。
type ResolvedPage struct {
	Page     *Page
	ViaAlias bool
}

// 搜索结果 matched_on 取值：命中的是页面标题还是别名。
const (
	// MatchedOnTitle 命中页面规范化标题。
	MatchedOnTitle = "title"
	// MatchedOnAlias 命中 page_alias（如改名留下的旧标题）。
	MatchedOnAlias = "alias"
)

// PageSearchHit 页面搜索结果条目（Service.SearchPages，编辑器引用选择器的后端）。
type PageSearchHit struct {
	ID           uuid.UUID
	DisplayTitle string
	NamespaceKey string
	MatchedOn    string
}

// Revision 可见性（revision.visibility）。
const (
	// VisibilityPublic 默认可见性，M1 仅有公开版本。
	VisibilityPublic = "public"
)

// 审计与 Outbox 事件常量（设计 §16、ADR-0003）。
const (
	// AggregateTypePage 页面聚合类型（audit_event/outbox_event 的 aggregate_type）。
	AggregateTypePage = "page"
	// EventTypeRevisionPublished 审计事件：Revision 发布。
	EventTypeRevisionPublished = "revision.published"
	// EventTypeRevisionRolledBack 审计事件：回滚产生新 Revision（payload 含 rolled_back_to）。
	EventTypeRevisionRolledBack = "revision.rolled_back"
	// EventTypePageCreated 审计事件：页面创建（M3-T04）。
	EventTypePageCreated = "page.created"
	// EventTypePageRenamed 审计事件：页面改名（payload 含 old_normalized_title，M3-T04）。
	EventTypePageRenamed = "page.renamed"
	// OutboxEventRevisionPublished Outbox 事件：页面 Revision 已发布，驱动投影重建。
	OutboxEventRevisionPublished = "page.revision_published"
	// OutboxEventPageCreated Outbox 事件：页面已创建，驱动未解析链接 Resolver（M3-T04，设计 §5.2）。
	OutboxEventPageCreated = "page.created"
	// OutboxEventPageRenamed Outbox 事件：页面已改名，驱动未解析链接 Resolver（M3-T04，设计 §5.2）。
	OutboxEventPageRenamed = "page.renamed"
)

// Revision 一次正式发布的页面版本（对应 revision 表，发布后不可变）。
// ContentHash/SchemaVersion 冗余自关联的 content_snapshot，供 API 响应使用，非表列。
type Revision struct {
	ID                uuid.UUID
	PageID            uuid.UUID
	ParentRevisionID  *uuid.UUID
	ContentSnapshotID uuid.UUID
	ActorID           uuid.UUID
	ChangeBatchID     *uuid.UUID
	Summary           string
	IsMinor           bool
	Visibility        string
	CreatedAt         time.Time

	ContentHash   string
	SchemaVersion int
}

// ContentSnapshot 某次 Revision 的完整 AST 快照（对应 content_snapshot 表，发布后不可变）。
// AST 为 canonical 序列化字节（ast.CanonicalizeJSON），ContentHash/SizeBytes 由服务端计算。
type ContentSnapshot struct {
	ID            uuid.UUID
	SchemaVersion int
	AST           json.RawMessage
	ContentHash   string
	SizeBytes     int
}

// AuditEvent 审计事件（对应 audit_event 表，只增不改）。
type AuditEvent struct {
	ID            uuid.UUID
	ActorID       uuid.UUID
	EventType     string
	AggregateType string
	AggregateID   uuid.UUID
	ChangeBatchID *uuid.UUID
	Payload       json.RawMessage
}

// OutboxEvent Outbox 事件（对应 outbox_event 表，status 默认 'pending'，
// next_attempt_at 默认 now()，由 Worker 领取消费）。
type OutboxEvent struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	Payload       json.RawMessage
}
