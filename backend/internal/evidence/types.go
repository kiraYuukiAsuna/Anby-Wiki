// Package evidence 实现 Evidence 领域服务：
//   - Asset（M4-T04）：媒体资产上传（StoreAsset）——流式 SHA-256 内容寻址写入对象存储，
//     asset + asset_revision 同事务落库，同 asset 同内容重复上传去重；
//   - Source/Citation（M4-T05）：URL 规范化（NormalizeURL）与
//     UpsertExternalResource / CreateSource / AddSourceVersion / CreateCitation，
//     是 external_resource / source / source_version / source_chunk / citation
//     的唯一权威写入入口（INV-07 定位侧）。
//
// 相关表结构见 migrations/000001_initial_schema.up.sql。
package evidence

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// 领域错误：哨兵 error，调用方用 errors.Is 判定；具体上下文通过 %w 包装附加。
var (
	// ErrAssetNotFound 按 (wiki_id, name) 未命中 active 状态的 asset。
	ErrAssetNotFound = errors.New("evidence: asset 不存在")
	// ErrAssetRevisionNotFound 同 asset 下未命中指定 content_hash 的 asset_revision。
	ErrAssetRevisionNotFound = errors.New("evidence: asset_revision 不存在")
	// ErrDuplicateAssetName 同 wiki 内同名 active asset 已存在（并发创建的兜底，
	// 正常路径在事务内先查后插，命中唯一索引说明有并发写）。
	ErrDuplicateAssetName = errors.New("evidence: asset 名字冲突")
	// ErrInvalidAssetInput 入参非法：name / mime_type 为空，或 content 为 nil。
	ErrInvalidAssetInput = errors.New("evidence: 资产入参非法")
)

// asset 状态（asset.status）。
const (
	// AssetStatusActive 正常资产。
	AssetStatusActive = "active"
	// AssetStatusDeleted 软删除（名字可被新资产复用）。
	AssetStatusDeleted = "deleted"
)

// DomainAsset 对象键的 domain 段（ADR-0004：{env}/{domain}/{hash前2位}/{hash}）。
const DomainAsset = "asset"

// Asset 逻辑媒体资产（asset 表）。
type Asset struct {
	ID                uuid.UUID
	WikiID            uuid.UUID
	Name              string
	CurrentRevisionID *uuid.UUID
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// AssetRevision 资产内容版本（asset_revision 表，不可变）。
type AssetRevision struct {
	ID           uuid.UUID
	AssetID      uuid.UUID
	StorageKey   string
	ContentHash  string
	MimeType     string
	SizeBytes    int64
	Width        *int32
	Height       *int32
	MetadataJSON []byte
	ActorID      uuid.UUID
	CreatedAt    time.Time
}
