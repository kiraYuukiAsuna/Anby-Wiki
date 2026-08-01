package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Source/Citation 域的领域错误（M4-T05）。哨兵 error，errors.Is 判定。
var (
	// ErrInvalidURL 输入不是合法的 http/https 绝对 URL（NormalizeURL）。
	ErrInvalidURL = errors.New("evidence: URL 非法")
	// ErrExternalResourceNotFound 按 normalized_url 或 ID 未命中 external_resource。
	ErrExternalResourceNotFound = errors.New("evidence: external_resource 不存在")
	// ErrExternalResourceExists normalized_url 唯一索引冲突（并发创建的兜底，
	// 服务层转为回查既有行幂等返回）。
	ErrExternalResourceExists = errors.New("evidence: external_resource 已存在")
	// ErrExternalResourceLeaseLost 表示健康检查租约已被后续领取者替换。
	ErrExternalResourceLeaseLost = errors.New("evidence: external_resource 检查租约已失效")
	// ErrSourceNotFound 按 ID 未命中 source。
	ErrSourceNotFound = errors.New("evidence: source 不存在")
	// ErrSourceVersionNotFound 按 ID 或 (source_id, version_hash) 未命中 source_version。
	ErrSourceVersionNotFound = errors.New("evidence: source_version 不存在")
	// ErrSourceVersionExists (source_id, version_hash) 唯一索引冲突（并发重复导入的
	// 兜底，服务层转为回查既有版本幂等返回）。
	ErrSourceVersionExists = errors.New("evidence: source_version 已存在")
	// ErrSourceChunkNotFound 按 ID 未命中 source_chunk。
	ErrSourceChunkNotFound = errors.New("evidence: source_chunk 不存在")
	// ErrInvalidSourceInput 来源入参非法：source_type/title/version_hash/fetched_at/
	// chunk 文本等不满足约束。
	ErrInvalidSourceInput = errors.New("evidence: 来源入参非法")
	// ErrInvalidChunkOrdinal chunk ordinal 必须从 0 开始连续递增。
	ErrInvalidChunkOrdinal = errors.New("evidence: chunk ordinal 必须从 0 开始连续")
	// ErrInvalidLocator locator 形态非法（页码、文本范围或图片/OCR 区域约束）。
	ErrInvalidLocator = errors.New("evidence: locator 非法")
	// ErrChunkVersionMismatch citation 指向的 chunk 不属于其 source_version（跨版本拒绝）。
	ErrChunkVersionMismatch = errors.New("evidence: chunk 与 citation 的 source_version 不一致")
	// ErrQuotationMismatch quotation 不是所定位 chunk 文本的子串（严格拒绝）。
	ErrQuotationMismatch = errors.New("evidence: quotation 不是 chunk 文本的子串")
	// ErrCitationNotFound 按 ID 未命中 citation。
	ErrCitationNotFound = errors.New("evidence: citation 不存在")
	// ErrInvalidSourceCursor 来源、版本或分片目录 cursor 解析失败。
	ErrInvalidSourceCursor = errors.New("evidence: 来源 cursor 非法")
)

// source_type 枚举（与 000007 CHECK 约束同值）。
const (
	// SourceTypeWebpage 网页。
	SourceTypeWebpage = "webpage"
	// SourceTypePDF PDF 文档。
	SourceTypePDF = "pdf"
	// SourceTypeBook 书籍。
	SourceTypeBook = "book"
	// SourceTypeImage 图片。
	SourceTypeImage = "image"
	// SourceTypeVideo 视频。
	SourceTypeVideo = "video"
	// SourceTypeAPI API 返回。
	SourceTypeAPI = "api"
	// SourceTypeDatabase 数据库记录。
	SourceTypeDatabase = "database"
)

// IsValidSourceType source_type 枚举校验。
func IsValidSourceType(t string) bool {
	switch t {
	case SourceTypeWebpage, SourceTypePDF, SourceTypeBook, SourceTypeImage,
		SourceTypeVideo, SourceTypeAPI, SourceTypeDatabase:
		return true
	}
	return false
}

// external_resource 状态（与 000007 CHECK 约束同值）。
const (
	ExternalResourceStatusUnknown  = "unknown"
	ExternalResourceStatusOK       = "ok"
	ExternalResourceStatusRedirect = "redirect"
	ExternalResourceStatusBroken   = "broken"
	ExternalResourceStatusBlocked  = "blocked"
)

// IsValidExternalResourceStatus 校验 external_resource.status 枚举。
func IsValidExternalResourceStatus(status string) bool {
	switch status {
	case ExternalResourceStatusUnknown, ExternalResourceStatusOK,
		ExternalResourceStatusRedirect, ExternalResourceStatusBroken,
		ExternalResourceStatusBlocked:
		return true
	}
	return false
}

const ImageRegionUnitPixel = "pixel"

// ImageRegion 保存图片或 PDF 栅格页中的像素区域。原图尺寸一并固化，避免后续
// 缩放展示时丢失可还原的定位坐标。
type ImageRegion struct {
	X           int32
	Y           int32
	Width       int32
	Height      int32
	ImageWidth  int32
	ImageHeight int32
	Unit        string
}

// OCRInfo 说明该定位文本来自 OCR，而不是来源自带的文本层。
type OCRInfo struct {
	Engine     string
	Languages  []string
	Confidence *float64
}

// Locator 来源定位信息（source_chunk.locator_json / citation.locator_json 的结构，
// 设计 §7.4/§7.5）。全部字段可选：page 页码（>=1）、section 章节名（非空）、
// char_start/char_end 文本字符范围，以及图片像素区域和 OCR 元数据。
// citation.locator 与 chunk.locator 叠加表示更细粒度定位
// （如 chunk 定位到页，citation 定位到页内字符段）。
type Locator struct {
	Page        *int32
	Section     *string
	CharStart   *int32
	CharEnd     *int32
	ImageRegion *ImageRegion
	OCR         *OCRInfo
}

// Validate 校验 locator 形态（Go 侧手写结构校验，等价契约层 Zod 校验）。
func (l Locator) Validate() error {
	if l.Page != nil && *l.Page < 1 {
		return fmt.Errorf("%w: page=%d（必须 >= 1）", ErrInvalidLocator, *l.Page)
	}
	if l.Section != nil && strings.TrimSpace(*l.Section) == "" {
		return fmt.Errorf("%w: section 为空", ErrInvalidLocator)
	}
	if l.CharStart != nil && *l.CharStart < 0 {
		return fmt.Errorf("%w: char_start=%d（必须 >= 0）", ErrInvalidLocator, *l.CharStart)
	}
	if l.CharEnd != nil {
		if *l.CharEnd < 0 {
			return fmt.Errorf("%w: char_end=%d（必须 >= 0）", ErrInvalidLocator, *l.CharEnd)
		}
		if l.CharStart != nil && *l.CharEnd < *l.CharStart {
			return fmt.Errorf("%w: char_end=%d < char_start=%d", ErrInvalidLocator, *l.CharEnd, *l.CharStart)
		}
	}
	if l.ImageRegion != nil {
		region := l.ImageRegion
		if region.Unit != ImageRegionUnitPixel ||
			region.X < 0 || region.Y < 0 ||
			region.Width <= 0 || region.Height <= 0 ||
			region.ImageWidth <= 0 || region.ImageHeight <= 0 ||
			int64(region.X)+int64(region.Width) > int64(region.ImageWidth) ||
			int64(region.Y)+int64(region.Height) > int64(region.ImageHeight) {
			return fmt.Errorf("%w: image_region 超出原图边界", ErrInvalidLocator)
		}
	}
	if l.OCR != nil {
		if l.ImageRegion == nil || strings.TrimSpace(l.OCR.Engine) == "" ||
			len(l.OCR.Languages) == 0 {
			return fmt.Errorf("%w: OCR 定位缺少图片区域、引擎或语言", ErrInvalidLocator)
		}
		seen := make(map[string]struct{}, len(l.OCR.Languages))
		for _, language := range l.OCR.Languages {
			language = strings.TrimSpace(language)
			if language == "" {
				return fmt.Errorf("%w: OCR language 为空", ErrInvalidLocator)
			}
			if _, exists := seen[language]; exists {
				return fmt.Errorf("%w: OCR language 重复", ErrInvalidLocator)
			}
			seen[language] = struct{}{}
		}
		if l.OCR.Confidence != nil &&
			(*l.OCR.Confidence < 0 || *l.OCR.Confidence > 1) {
			return fmt.Errorf("%w: OCR confidence 必须在 0..1", ErrInvalidLocator)
		}
	}
	return nil
}

// toJSON 序列化为只含已设字段的 JSON object；全空时为 "{}"（Validate 已通过为前提）。
func (l Locator) toJSON() []byte {
	m := map[string]any{}
	if l.Page != nil {
		m["page"] = *l.Page
	}
	if l.Section != nil {
		m["section"] = *l.Section
	}
	if l.CharStart != nil {
		m["char_start"] = *l.CharStart
	}
	if l.CharEnd != nil {
		m["char_end"] = *l.CharEnd
	}
	if l.ImageRegion != nil {
		m["image_region"] = map[string]any{
			"x":            l.ImageRegion.X,
			"y":            l.ImageRegion.Y,
			"width":        l.ImageRegion.Width,
			"height":       l.ImageRegion.Height,
			"image_width":  l.ImageRegion.ImageWidth,
			"image_height": l.ImageRegion.ImageHeight,
			"unit":         l.ImageRegion.Unit,
		}
	}
	if l.OCR != nil {
		ocr := map[string]any{
			"engine":    l.OCR.Engine,
			"languages": l.OCR.Languages,
		}
		if l.OCR.Confidence != nil {
			ocr["confidence"] = *l.OCR.Confidence
		}
		m["ocr"] = ocr
	}
	b, err := json.Marshal(m)
	if err != nil {
		// 字段类型受控，不可达；防御性返回空 object。
		return []byte("{}")
	}
	return b
}

// ExternalResource 规范化外部资源（external_resource 表，设计 §7.1）。
// 普通外链与引用来源共享同一行。
type ExternalResource struct {
	ID                  uuid.UUID
	OriginalURL         string
	NormalizedURL       string
	CanonicalURL        *string
	Domain              string
	Path                string
	HTTPStatus          *int32
	ContentHash         *string
	Status              string
	RedirectTargetID    *uuid.UUID
	LastCheckedAt       *time.Time
	LastSuccessAt       *time.Time
	NextCheckAt         time.Time
	LeaseToken          *uuid.UUID
	ConsecutiveFailures int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Source 逻辑来源（source 表，设计 §7.2）。不等同于某次抓取结果；
// external_resource_id / asset_id 允许皆空（先登记元数据后关联）。
type Source struct {
	ID                 uuid.UUID
	SourceType         string
	ExternalResourceID *uuid.UUID
	AssetID            *uuid.UUID
	Title              string
	Author             *string
	Publisher          *string
	PublishedAt        *time.Time
	MetadataJSON       []byte
	CreatedAt          time.Time
}

// SourceVersion 来源某时间点的具体版本（source_version 表，不可变，设计 §7.3）。
// VersionHash 是客户端内容哈希（如抓取内容 SHA-256），
// unique(source_id, version_hash) 是重复导入不重复抽取的 DB 基础。
type SourceVersion struct {
	ID               uuid.UUID
	SourceID         uuid.UUID
	VersionHash      string
	RawAssetID       *uuid.UUID
	ExtractedAssetID *uuid.UUID
	FetchedAt        time.Time
	CreatedAt        time.Time
}

// SourceChunk SourceVersion 的可定位分片（source_chunk 表，不可变，设计 §7.4）。
// TextHash 为服务端计算的 text_content 小写 hex SHA-256。
type SourceChunk struct {
	ID              uuid.UUID
	SourceVersionID uuid.UUID
	Ordinal         int
	LocatorJSON     []byte
	TextContent     string
	TextHash        string
	CreatedAt       time.Time
}

// Citation 指向 SourceVersion 中特定位置的证据引用（citation 表，不可变，设计 §7.5）。
// 定位粒度：source_version 必有；source_chunk_id 可选（必须属于该 version）；
// locator_json 可选（与 chunk locator 叠加的更细粒度定位）；
// quotation 可选（提供时 quotation_hash = SHA-256(quotation)，
// 且若定位到 chunk 必须是 chunk 文本的子串）。
type Citation struct {
	ID              uuid.UUID
	SourceVersionID uuid.UUID
	SourceChunkID   *uuid.UUID
	LocatorJSON     []byte
	Quotation       *string
	QuotationHash   *string
	CreatedBy       uuid.UUID
	CreatedAt       time.Time
}

// CitationDetail 是只读详情页所需的完整证据定位链。
type CitationDetail struct {
	Citation         Citation
	SourceVersion    SourceVersion
	Source           Source
	SourceChunk      *SourceChunk
	ExternalResource *ExternalResource
}

// SourcePage 是来源目录的稳定游标分页结果。
type SourcePage struct {
	Items      []Source
	NextCursor *string
}

// SourceVersionPage 是某一来源不可变版本的稳定游标分页结果。
type SourceVersionPage struct {
	Items      []SourceVersion
	NextCursor *string
}

// SourceChunkPage 是某一不可变来源版本分片的稳定游标分页结果。
type SourceChunkPage struct {
	Items      []SourceChunk
	NextCursor *string
}

// SourceDetail 是来源工作台所需的逻辑来源及其规范化外部资源。
type SourceDetail struct {
	Source           Source
	ExternalResource *ExternalResource
}
