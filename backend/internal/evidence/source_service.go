package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// source_service.go —— Source/Citation 域服务（M4-T05，设计 §7.1-7.5）。
// 与 asset 域共用 Service 装配（repo/pages/store/env/txm/ids），
// UpsertExternalResource / CreateSource / AddSourceVersion / CreateCitation
// 是 external_resource / source / source_version / source_chunk / citation
// 的唯一权威写入入口。

// sha256Hex 小写 hex SHA-256（chunk text_hash / citation quotation_hash）。
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// CreateSourceParams 创建来源的入参。
// 关联外部资源两种方式：URL（自动 UpsertExternalResource 关联，优先）或
// ExternalResourceID（必须已存在）；两者皆空则先只登记元数据。
// Author/Publisher 空白归 nil；Metadata 空落库 '{}'，非空必须是 JSON object。
type CreateSourceParams struct {
	SourceType         string
	ExternalResourceID *uuid.UUID
	URL                string
	AssetID            *uuid.UUID
	Title              string
	Author             string
	Publisher          string
	PublishedAt        *time.Time
	Metadata           json.RawMessage
	ActorID            uuid.UUID
}

// CreateSource 创建逻辑来源（source 表的唯一权威写入入口）。
// 校验链：Actor 准入 → source_type 枚举 → title 非空 → 关联对象存在性
// （URL 自动 upsert / ExternalResourceID 存在 / AssetID 存在且 active）。
func (s *Service) CreateSource(ctx context.Context, params CreateSourceParams) (*Source, error) {
	if err := s.pages.CheckWriteActor(ctx, nil, params.ActorID); err != nil {
		return nil, err
	}
	if !IsValidSourceType(params.SourceType) {
		return nil, fmt.Errorf("%w: source_type=%q", ErrInvalidSourceInput, params.SourceType)
	}
	title := strings.TrimSpace(params.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: title 为空", ErrInvalidSourceInput)
	}

	var resourceID *uuid.UUID
	if rawURL := strings.TrimSpace(params.URL); rawURL != "" {
		e, err := s.UpsertExternalResource(ctx, rawURL)
		if err != nil {
			return nil, err
		}
		resourceID = &e.ID
	} else if params.ExternalResourceID != nil {
		if _, err := s.repo.GetExternalResourceByID(ctx, nil, *params.ExternalResourceID); err != nil {
			return nil, err
		}
		resourceID = params.ExternalResourceID
	}
	if params.AssetID != nil {
		if _, err := s.repo.GetActiveAssetByID(ctx, nil, *params.AssetID); err != nil {
			return nil, err
		}
	}

	metadata := params.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	} else {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(metadata, &obj); err != nil || obj == nil {
			return nil, fmt.Errorf("%w: metadata 必须是 JSON object", ErrInvalidSourceInput)
		}
	}
	var author, publisher *string
	if a := strings.TrimSpace(params.Author); a != "" {
		author = &a
	}
	if p := strings.TrimSpace(params.Publisher); p != "" {
		publisher = &p
	}

	sourceID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	src := &Source{
		ID:                 sourceID,
		SourceType:         params.SourceType,
		ExternalResourceID: resourceID,
		AssetID:            params.AssetID,
		Title:              title,
		Author:             author,
		Publisher:          publisher,
		PublishedAt:        params.PublishedAt,
		MetadataJSON:       metadata,
	}
	if err := s.repo.InsertSource(ctx, nil, src); err != nil {
		return nil, err
	}
	return src, nil
}

// ChunkInput 来源分片入参。Ordinal 必须从 0 开始连续递增；
// Locator 全字段可选；TextContent 必须非空。
type ChunkInput struct {
	Ordinal     int
	Locator     Locator
	TextContent string
}

// AddSourceVersionParams 登记来源版本的入参。
// VersionHash 是客户端内容哈希（如抓取内容的 SHA-256）。
type AddSourceVersionParams struct {
	SourceID    uuid.UUID
	VersionHash string
	RawAssetID  *uuid.UUID
	FetchedAt   time.Time
	Chunks      []ChunkInput
}

// AddSourceVersionResult AddSourceVersion 的结果。
type AddSourceVersionResult struct {
	Version *SourceVersion
	Chunks  []SourceChunk
	// Reused 为 true 表示 version_hash 重复：幂等返回既有版本与其 chunks，
	// 未重复插入（M6 重复导入不重复抽取的 DB 基础）。
	Reused bool
}

// AddSourceVersion 登记来源版本与其分片（source_version + source_chunk 同事务写入）。
// 校验链：version_hash 非空 → fetched_at 非零 → source 存在 → raw_asset 存在 →
// chunks ordinal 从 0 连续 / 文本非空 / locator 形态。
// version_hash 重复（含并发撞唯一索引）幂等返回既有版本（Reused=true）。
// chunk text_hash 由服务端按 text_content 计算（SHA-256 hex），不信任调用方。
//
// source_version 表无 actor 列（版本登记属导入管道行为），故本方法不做 Actor 准入。
func (s *Service) AddSourceVersion(ctx context.Context, params AddSourceVersionParams) (*AddSourceVersionResult, error) {
	versionHash := strings.TrimSpace(params.VersionHash)
	if versionHash == "" {
		return nil, fmt.Errorf("%w: version_hash 为空", ErrInvalidSourceInput)
	}
	if params.FetchedAt.IsZero() {
		return nil, fmt.Errorf("%w: fetched_at 为零值", ErrInvalidSourceInput)
	}
	if _, err := s.repo.GetSourceByID(ctx, nil, params.SourceID); err != nil {
		return nil, err
	}
	if params.RawAssetID != nil {
		if _, err := s.repo.GetAssetRevisionByID(ctx, nil, *params.RawAssetID); err != nil {
			return nil, err
		}
	}
	for i, c := range params.Chunks {
		if c.Ordinal != i {
			return nil, fmt.Errorf("%w: 第 %d 个 chunk ordinal=%d", ErrInvalidChunkOrdinal, i, c.Ordinal)
		}
		if c.TextContent == "" {
			return nil, fmt.Errorf("%w: 第 %d 个 chunk 文本为空", ErrInvalidSourceInput, i)
		}
		if err := c.Locator.Validate(); err != nil {
			return nil, err
		}
	}

	var result *AddSourceVersionResult
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		// 事务内查重：并发重复导入恰一插入，后到者复用既有版本。
		existing, err := s.repo.GetSourceVersionByHash(ctx, tx, params.SourceID, versionHash)
		if err == nil {
			chunks, err := s.repo.ListSourceChunks(ctx, tx, existing.ID)
			if err != nil {
				return err
			}
			result = &AddSourceVersionResult{Version: existing, Chunks: chunks, Reused: true}
			return nil
		}
		if !errors.Is(err, ErrSourceVersionNotFound) {
			return err
		}

		versionID, err := s.ids.New()
		if err != nil {
			return err
		}
		v := &SourceVersion{
			ID:          versionID,
			SourceID:    params.SourceID,
			VersionHash: versionHash,
			RawAssetID:  params.RawAssetID,
			FetchedAt:   params.FetchedAt,
		}
		if err := s.repo.InsertSourceVersion(ctx, tx, v); err != nil {
			return err
		}
		chunks := make([]SourceChunk, 0, len(params.Chunks))
		for _, input := range params.Chunks {
			chunkID, err := s.ids.New()
			if err != nil {
				return err
			}
			c := SourceChunk{
				ID:              chunkID,
				SourceVersionID: versionID,
				Ordinal:         input.Ordinal,
				LocatorJSON:     input.Locator.toJSON(),
				TextContent:     input.TextContent,
				TextHash:        sha256Hex(input.TextContent),
			}
			if err := s.repo.InsertSourceChunk(ctx, tx, &c); err != nil {
				return err
			}
			chunks = append(chunks, c)
		}
		result = &AddSourceVersionResult{Version: v, Chunks: chunks}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrSourceVersionExists) {
			// 并发插入撞 (source_id, version_hash) 唯一索引：回查既有版本，幂等返回。
			existing, qerr := s.repo.GetSourceVersionByHash(ctx, nil, params.SourceID, versionHash)
			if qerr != nil {
				return nil, err
			}
			chunks, qerr := s.repo.ListSourceChunks(ctx, nil, existing.ID)
			if qerr != nil {
				return nil, qerr
			}
			return &AddSourceVersionResult{Version: existing, Chunks: chunks, Reused: true}, nil
		}
		return nil, err
	}
	return result, nil
}

// CreateCitationParams 创建证据引用的入参。
// SourceChunkID 可选但必须属于 SourceVersionID；Locator 可选（与 chunk locator
// 叠加的更细粒度定位）；Quotation 可选（空串视为未提供）。
type CreateCitationParams struct {
	SourceVersionID uuid.UUID
	SourceChunkID   *uuid.UUID
	Locator         *Locator
	Quotation       string
	ActorID         uuid.UUID
}

// CreateCitation 创建证据引用（citation 表的唯一权威写入入口，INV-07 定位侧）。
// 校验链：Actor 准入 → source_version 存在 → chunk 存在且属于该 version
// （跨版本拒绝）→ quotation 非空时是 chunk 文本子串（严格拒绝，见下）→
// locator 形态。quotation_hash = SHA-256(quotation) 由服务端计算。
//
// quotation 子串校验是严格拒绝而非警告：citation 是权威证据数据，
// 无法在定位文本中复核的引文不应落库（宽松警告会把无法核验的引文变成权威数据）。
func (s *Service) CreateCitation(ctx context.Context, params CreateCitationParams) (*Citation, error) {
	if err := s.pages.CheckWriteActor(ctx, nil, params.ActorID); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetSourceVersionByID(ctx, nil, params.SourceVersionID); err != nil {
		return nil, err
	}
	var chunk *SourceChunk
	if params.SourceChunkID != nil {
		c, err := s.repo.GetSourceChunkByID(ctx, nil, *params.SourceChunkID)
		if err != nil {
			return nil, err
		}
		if c.SourceVersionID != params.SourceVersionID {
			return nil, fmt.Errorf("%w: chunk=%s 属于 version=%s，citation 指向 %s",
				ErrChunkVersionMismatch, c.ID, c.SourceVersionID, params.SourceVersionID)
		}
		chunk = c
	}

	var quotation, quotationHash *string
	if q := params.Quotation; q != "" {
		if chunk != nil && !strings.Contains(chunk.TextContent, q) {
			return nil, fmt.Errorf("%w: chunk=%s", ErrQuotationMismatch, chunk.ID)
		}
		hash := sha256Hex(q)
		quotation = &q
		quotationHash = &hash
	}

	var locatorJSON []byte
	if params.Locator != nil {
		if err := params.Locator.Validate(); err != nil {
			return nil, err
		}
		locatorJSON = params.Locator.toJSON()
	}

	citationID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	c := &Citation{
		ID:              citationID,
		SourceVersionID: params.SourceVersionID,
		SourceChunkID:   params.SourceChunkID,
		LocatorJSON:     locatorJSON,
		Quotation:       quotation,
		QuotationHash:   quotationHash,
		CreatedBy:       params.ActorID,
	}
	if err := s.repo.InsertCitation(ctx, nil, c); err != nil {
		return nil, err
	}
	return c, nil
}

// GetCitationDetail 沿不可变 Citation → SourceVersion → Source（→ Chunk/URL）
// 读取完整定位链，供 API/UI 只读展示；不写任何权威或投影状态。
func (s *Service) GetCitationDetail(ctx context.Context, citationID uuid.UUID) (*CitationDetail, error) {
	citation, err := s.repo.GetCitationByID(ctx, nil, citationID)
	if err != nil {
		return nil, err
	}
	version, err := s.repo.GetSourceVersionByID(ctx, nil, citation.SourceVersionID)
	if err != nil {
		return nil, err
	}
	source, err := s.repo.GetSourceByID(ctx, nil, version.SourceID)
	if err != nil {
		return nil, err
	}
	detail := &CitationDetail{Citation: *citation, SourceVersion: *version, Source: *source}
	if citation.SourceChunkID != nil {
		chunk, err := s.repo.GetSourceChunkByID(ctx, nil, *citation.SourceChunkID)
		if err != nil {
			return nil, err
		}
		detail.SourceChunk = chunk
	}
	if source.ExternalResourceID != nil {
		resource, err := s.repo.GetExternalResourceByID(ctx, nil, *source.ExternalResourceID)
		if err != nil {
			return nil, err
		}
		detail.ExternalResource = resource
	}
	return detail, nil
}
