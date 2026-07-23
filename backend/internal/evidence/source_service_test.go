package evidence_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/testkit"
)

// createSource 经服务创建一个最小 webpage 来源（URL 自动 upsert 关联）。
func createSource(t *testing.T, svc *evidence.Service, actor uuid.UUID) *evidence.Source {
	t.Helper()
	src, err := svc.CreateSource(context.Background(), evidence.CreateSourceParams{
		SourceType: evidence.SourceTypeWebpage,
		URL:        "https://example.com/articles/anby",
		Title:      "安比·德玛拉介绍",
		ActorID:    actor,
	})
	if err != nil {
		t.Fatalf("CreateSource 失败: %v", err)
	}
	return src
}

func TestUpsertExternalResource_Idempotent(t *testing.T) {
	_, svc, _ := setup(t)
	ctx := context.Background()

	raw1 := "HTTP://Example.COM:80/a/b/?utm_source=x&id=1#frag"
	raw2 := "http://example.com/a/b?id=1"
	e1, err := svc.UpsertExternalResource(ctx, raw1)
	if err != nil {
		t.Fatalf("首次 Upsert 失败: %v", err)
	}
	if e1.NormalizedURL != "http://example.com/a/b?id=1" {
		t.Fatalf("normalized_url = %q，期望 %q", e1.NormalizedURL, "http://example.com/a/b?id=1")
	}
	if e1.OriginalURL != raw1 {
		t.Fatalf("original_url = %q，期望保留输入原文 %q", e1.OriginalURL, raw1)
	}
	if e1.Domain != "example.com" || e1.Path != "/a/b" {
		t.Fatalf("domain/path = %q/%q，期望 example.com//a/b", e1.Domain, e1.Path)
	}
	if e1.Status != evidence.ExternalResourceStatusUnknown {
		t.Fatalf("status = %q，期望 unknown", e1.Status)
	}

	// 规范化等价的不同原文：返回既有行（幂等查重）。
	e2, err := svc.UpsertExternalResource(ctx, raw2)
	if err != nil {
		t.Fatalf("重复 Upsert 失败: %v", err)
	}
	if e2.ID != e1.ID {
		t.Fatalf("重复 Upsert 返回新行 %s，期望复用 %s", e2.ID, e1.ID)
	}
	if e2.OriginalURL != raw1 {
		t.Fatalf("既有行 original_url = %q，期望保留首次输入 %q", e2.OriginalURL, raw1)
	}
}

func TestUpsertExternalResource_InvalidURL(t *testing.T) {
	_, svc, _ := setup(t)
	if _, err := svc.UpsertExternalResource(context.Background(), "not-a-url"); !errors.Is(err, evidence.ErrInvalidURL) {
		t.Fatalf("err = %v，期望 ErrInvalidURL", err)
	}
}

func TestCreateSource_LinkModes(t *testing.T) {
	tdb, svc, _ := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	// 方式一：URL 自动 UpsertExternalResource 关联。
	byURL, err := svc.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType: evidence.SourceTypeWebpage,
		URL:        "HTTPS://Example.COM:443/reports/zzz?utm_campaign=launch",
		Title:      "ZZZ 报告",
		Author:     "  某作者  ",
		Metadata:   json.RawMessage(`{"lang":"zh"}`),
		ActorID:    actor,
	})
	if err != nil {
		t.Fatalf("URL 关联 CreateSource 失败: %v", err)
	}
	if byURL.ExternalResourceID == nil {
		t.Fatalf("URL 关联应回填 external_resource_id")
	}
	er, err := svc.UpsertExternalResource(ctx, "https://example.com/reports/zzz")
	if err != nil {
		t.Fatalf("回查 external_resource 失败: %v", err)
	}
	if *byURL.ExternalResourceID != er.ID {
		t.Fatalf("external_resource_id = %s，期望 %s", *byURL.ExternalResourceID, er.ID)
	}
	if byURL.Author == nil || *byURL.Author != "某作者" {
		t.Fatalf("author = %v，期望 trim 后 %q", byURL.Author, "某作者")
	}

	// 方式二：显式 ExternalResourceID（必须已存在）。
	byID, err := svc.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType:         evidence.SourceTypePDF,
		ExternalResourceID: &er.ID,
		Title:              "同一资源的 PDF 来源",
		ActorID:            actor,
	})
	if err != nil {
		t.Fatalf("ExternalResourceID 关联 CreateSource 失败: %v", err)
	}
	if byID.ExternalResourceID == nil || *byID.ExternalResourceID != er.ID {
		t.Fatalf("external_resource_id = %v，期望 %s", byID.ExternalResourceID, er.ID)
	}

	// 不存在的 ExternalResourceID 拒绝。
	missing := tdb.NewID(t)
	if _, err := svc.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType:         evidence.SourceTypePDF,
		ExternalResourceID: &missing,
		Title:              "x",
		ActorID:            actor,
	}); !errors.Is(err, evidence.ErrExternalResourceNotFound) {
		t.Fatalf("err = %v，期望 ErrExternalResourceNotFound", err)
	}

	// 方式三：AssetID 关联（StoreAsset 先建资产）。
	asset, err := svc.StoreAsset(ctx, evidence.StoreAssetParams{
		WikiID:   testkit.DefaultWikiID,
		Name:     "cover.png",
		Content:  strings.NewReader("png-bytes"),
		MimeType: "image/png",
		ActorID:  actor,
	})
	if err != nil {
		t.Fatalf("StoreAsset 失败: %v", err)
	}
	byAsset, err := svc.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType: evidence.SourceTypeImage,
		AssetID:    &asset.Asset.ID,
		Title:      "封面图",
		ActorID:    actor,
	})
	if err != nil {
		t.Fatalf("AssetID 关联 CreateSource 失败: %v", err)
	}
	if byAsset.AssetID == nil || *byAsset.AssetID != asset.Asset.ID {
		t.Fatalf("asset_id = %v，期望 %s", byAsset.AssetID, asset.Asset.ID)
	}

	// 不存在的 AssetID 拒绝。
	missingAsset := tdb.NewID(t)
	if _, err := svc.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType: evidence.SourceTypeImage,
		AssetID:    &missingAsset,
		Title:      "x",
		ActorID:    actor,
	}); !errors.Is(err, evidence.ErrAssetNotFound) {
		t.Fatalf("err = %v，期望 ErrAssetNotFound", err)
	}
}

func TestCreateSource_Validation(t *testing.T) {
	tdb, svc, _ := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	// 非法 source_type / 空 title / 非 object metadata。
	if _, err := svc.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType: "rss", Title: "x", ActorID: actor,
	}); !errors.Is(err, evidence.ErrInvalidSourceInput) {
		t.Fatalf("source_type err = %v，期望 ErrInvalidSourceInput", err)
	}
	if _, err := svc.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType: evidence.SourceTypeWebpage, Title: "  ", ActorID: actor,
	}); !errors.Is(err, evidence.ErrInvalidSourceInput) {
		t.Fatalf("title err = %v，期望 ErrInvalidSourceInput", err)
	}
	if _, err := svc.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType: evidence.SourceTypeWebpage, Title: "x",
		Metadata: json.RawMessage(`[1,2]`), ActorID: actor,
	}); !errors.Is(err, evidence.ErrInvalidSourceInput) {
		t.Fatalf("metadata err = %v，期望 ErrInvalidSourceInput", err)
	}

	// Actor 准入：不存在的 actor / ai actor 拒绝。
	if _, err := svc.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType: evidence.SourceTypeWebpage, Title: "x",
		ActorID: uuid.MustParse("00000000-0000-7000-8000-999999999999"),
	}); !errors.Is(err, page.ErrInvalidActor) {
		t.Fatalf("不存在 actor err = %v，期望 ErrInvalidActor", err)
	}
	aiActor := tdb.MakeActor(t, "ai", "llm")
	if _, err := svc.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType: evidence.SourceTypeWebpage, Title: "x", ActorID: aiActor,
	}); !errors.Is(err, page.ErrActorNotAllowed) {
		t.Fatalf("ai actor err = %v，期望 ErrActorNotAllowed", err)
	}
}

// addVersion 快捷登记一个带 chunks 的来源版本。
func addVersion(t *testing.T, svc *evidence.Service, sourceID uuid.UUID, versionHash string, chunks []evidence.ChunkInput) *evidence.AddSourceVersionResult {
	t.Helper()
	res, err := svc.AddSourceVersion(context.Background(), evidence.AddSourceVersionParams{
		SourceID:    sourceID,
		VersionHash: versionHash,
		FetchedAt:   time.Now(),
		Chunks:      chunks,
	})
	if err != nil {
		t.Fatalf("AddSourceVersion(%q) 失败: %v", versionHash, err)
	}
	return res
}

func TestAddSourceVersion_Basic(t *testing.T) {
	tdb, svc, _ := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	src := createSource(t, svc, actor)

	page1 := int32(1)
	res := addVersion(t, svc, src.ID, "hash-v1", []evidence.ChunkInput{
		{Ordinal: 0, TextContent: "第一段文本", Locator: evidence.Locator{Page: &page1}},
		{Ordinal: 1, TextContent: "第二段文本"},
	})
	if res.Reused {
		t.Fatalf("首次登记 Reused = true，期望 false")
	}
	if res.Version.SourceID != src.ID || res.Version.VersionHash != "hash-v1" {
		t.Fatalf("version = %+v，字段不符合预期", res.Version)
	}
	if len(res.Chunks) != 2 {
		t.Fatalf("chunks 数 = %d，期望 2", len(res.Chunks))
	}
	for i, c := range res.Chunks {
		if c.Ordinal != i || c.SourceVersionID != res.Version.ID {
			t.Fatalf("chunk[%d] = %+v，ordinal/version 不符合预期", i, c)
		}
		if want := sha256Hex(c.TextContent); c.TextHash != want {
			t.Fatalf("chunk[%d] text_hash = %q，期望服务端计算 %q", i, c.TextHash, want)
		}
	}
	// locator_json 只含已设字段；全空为 '{}'。
	var loc map[string]any
	if err := json.Unmarshal(res.Chunks[0].LocatorJSON, &loc); err != nil {
		t.Fatalf("locator_json 解析失败: %v", err)
	}
	if len(loc) != 1 || loc["page"] != float64(1) {
		t.Fatalf("chunk[0] locator_json = %s，期望 {\"page\":1}", res.Chunks[0].LocatorJSON)
	}
	if string(res.Chunks[1].LocatorJSON) != "{}" {
		t.Fatalf("chunk[1] locator_json = %s，期望 {}", res.Chunks[1].LocatorJSON)
	}
}

func TestAddSourceVersion_Idempotent(t *testing.T) {
	tdb, svc, _ := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	src := createSource(t, svc, actor)

	chunks := []evidence.ChunkInput{{Ordinal: 0, TextContent: "内容"}, {Ordinal: 1, TextContent: "内容二"}}
	first := addVersion(t, svc, src.ID, "hash-dup", chunks)
	second := addVersion(t, svc, src.ID, "hash-dup", chunks)

	if !second.Reused {
		t.Fatalf("重复 version_hash Reused = false，期望 true")
	}
	if second.Version.ID != first.Version.ID {
		t.Fatalf("重复登记返回新版本 %s，期望复用 %s", second.Version.ID, first.Version.ID)
	}
	if len(second.Chunks) != 2 || second.Chunks[0].ID != first.Chunks[0].ID {
		t.Fatalf("重复登记 chunks = %+v，期望复用既有分片", second.Chunks)
	}

	// 不同 version_hash 正常新增。
	third := addVersion(t, svc, src.ID, "hash-v2", chunks)
	if third.Reused || third.Version.ID == first.Version.ID {
		t.Fatalf("新 version_hash 应新增版本，实际 %+v", third)
	}
}

func TestAddSourceVersion_Validation(t *testing.T) {
	tdb, svc, _ := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	src := createSource(t, svc, actor)
	ctx := context.Background()

	add := func(params evidence.AddSourceVersionParams) error {
		_, err := svc.AddSourceVersion(ctx, params)
		return err
	}

	// source 不存在。
	if err := add(evidence.AddSourceVersionParams{
		SourceID: tdb.NewID(t), VersionHash: "h", FetchedAt: time.Now(),
	}); !errors.Is(err, evidence.ErrSourceNotFound) {
		t.Fatalf("source 不存在 err = %v，期望 ErrSourceNotFound", err)
	}
	// version_hash 空 / fetched_at 零值。
	if err := add(evidence.AddSourceVersionParams{
		SourceID: src.ID, VersionHash: " ", FetchedAt: time.Now(),
	}); !errors.Is(err, evidence.ErrInvalidSourceInput) {
		t.Fatalf("version_hash err = %v，期望 ErrInvalidSourceInput", err)
	}
	if err := add(evidence.AddSourceVersionParams{
		SourceID: src.ID, VersionHash: "h",
	}); !errors.Is(err, evidence.ErrInvalidSourceInput) {
		t.Fatalf("fetched_at err = %v，期望 ErrInvalidSourceInput", err)
	}
	// ordinal 不从 0 开始 / 不连续。
	if err := add(evidence.AddSourceVersionParams{
		SourceID: src.ID, VersionHash: "h", FetchedAt: time.Now(),
		Chunks: []evidence.ChunkInput{{Ordinal: 1, TextContent: "x"}},
	}); !errors.Is(err, evidence.ErrInvalidChunkOrdinal) {
		t.Fatalf("ordinal 从 1 开始 err = %v，期望 ErrInvalidChunkOrdinal", err)
	}
	if err := add(evidence.AddSourceVersionParams{
		SourceID: src.ID, VersionHash: "h", FetchedAt: time.Now(),
		Chunks: []evidence.ChunkInput{{Ordinal: 0, TextContent: "x"}, {Ordinal: 2, TextContent: "y"}},
	}); !errors.Is(err, evidence.ErrInvalidChunkOrdinal) {
		t.Fatalf("ordinal 跳号 err = %v，期望 ErrInvalidChunkOrdinal", err)
	}
	// chunk 文本为空。
	if err := add(evidence.AddSourceVersionParams{
		SourceID: src.ID, VersionHash: "h", FetchedAt: time.Now(),
		Chunks: []evidence.ChunkInput{{Ordinal: 0, TextContent: ""}},
	}); !errors.Is(err, evidence.ErrInvalidSourceInput) {
		t.Fatalf("空文本 err = %v，期望 ErrInvalidSourceInput", err)
	}
	// locator 非法：char_end < char_start。
	start, end := int32(10), int32(5)
	if err := add(evidence.AddSourceVersionParams{
		SourceID: src.ID, VersionHash: "h", FetchedAt: time.Now(),
		Chunks: []evidence.ChunkInput{{
			Ordinal: 0, TextContent: "x",
			Locator: evidence.Locator{CharStart: &start, CharEnd: &end},
		}},
	}); !errors.Is(err, evidence.ErrInvalidLocator) {
		t.Fatalf("locator err = %v，期望 ErrInvalidLocator", err)
	}
	// raw_asset 不存在。
	missingAsset := tdb.NewID(t)
	if err := add(evidence.AddSourceVersionParams{
		SourceID: src.ID, VersionHash: "h", FetchedAt: time.Now(), RawAssetID: &missingAsset,
	}); !errors.Is(err, evidence.ErrAssetRevisionNotFound) {
		t.Fatalf("raw_asset err = %v，期望 ErrAssetRevisionNotFound", err)
	}
	// raw_asset 存在（asset_revision ID）时正常登记。
	asset, err := svc.StoreAsset(ctx, evidence.StoreAssetParams{
		WikiID:   testkit.DefaultWikiID,
		Name:     "raw.html",
		Content:  strings.NewReader("<html>raw</html>"),
		MimeType: "text/html",
		ActorID:  actor,
	})
	if err != nil {
		t.Fatalf("StoreAsset 失败: %v", err)
	}
	res, err := svc.AddSourceVersion(ctx, evidence.AddSourceVersionParams{
		SourceID: src.ID, VersionHash: "h-raw", FetchedAt: time.Now(), RawAssetID: &asset.Revision.ID,
	})
	if err != nil {
		t.Fatalf("带 raw_asset 登记失败: %v", err)
	}
	if res.Version.RawAssetID == nil || *res.Version.RawAssetID != asset.Revision.ID {
		t.Fatalf("raw_asset_id = %v，期望 %s", res.Version.RawAssetID, asset.Revision.ID)
	}
}

func TestCreateCitation_Basic(t *testing.T) {
	tdb, svc, _ := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	src := createSource(t, svc, actor)
	version := addVersion(t, svc, src.ID, "hash-v1", []evidence.ChunkInput{
		{Ordinal: 0, TextContent: "安比·德玛拉是《绝区零》的看板娘。"},
		{Ordinal: 1, TextContent: "第二段"},
	})
	chunk := version.Chunks[0]

	section := "角色介绍"
	c, err := svc.CreateCitation(context.Background(), evidence.CreateCitationParams{
		SourceVersionID: version.Version.ID,
		SourceChunkID:   &chunk.ID,
		Locator:         &evidence.Locator{Section: &section},
		Quotation:       "看板娘",
		ActorID:         actor,
	})
	if err != nil {
		t.Fatalf("CreateCitation 失败: %v", err)
	}
	if c.SourceVersionID != version.Version.ID || c.SourceChunkID == nil || *c.SourceChunkID != chunk.ID {
		t.Fatalf("citation = %+v，version/chunk 指向不符合预期", c)
	}
	if c.Quotation == nil || *c.Quotation != "看板娘" {
		t.Fatalf("quotation = %v，期望 %q", c.Quotation, "看板娘")
	}
	if want := sha256Hex("看板娘"); c.QuotationHash == nil || *c.QuotationHash != want {
		t.Fatalf("quotation_hash = %v，期望 %q", c.QuotationHash, want)
	}
	var loc map[string]any
	if err := json.Unmarshal(c.LocatorJSON, &loc); err != nil {
		t.Fatalf("locator_json 解析失败: %v", err)
	}
	if loc["section"] != "角色介绍" {
		t.Fatalf("locator_json = %s，期望含 section", c.LocatorJSON)
	}
	if c.CreatedBy != actor {
		t.Fatalf("created_by = %s，期望 %s", c.CreatedBy, actor)
	}

	// 不带 chunk / quotation 的版本级 citation：locator 与 hash 均为空。
	minimal, err := svc.CreateCitation(context.Background(), evidence.CreateCitationParams{
		SourceVersionID: version.Version.ID,
		ActorID:         actor,
	})
	if err != nil {
		t.Fatalf("最小 citation 失败: %v", err)
	}
	if minimal.SourceChunkID != nil || minimal.Quotation != nil || minimal.QuotationHash != nil || minimal.LocatorJSON != nil {
		t.Fatalf("最小 citation = %+v，可选字段应全空", minimal)
	}

	// 不带 chunk 的 quotation：无子串校验，只记 hash。
	noChunk, err := svc.CreateCitation(context.Background(), evidence.CreateCitationParams{
		SourceVersionID: version.Version.ID,
		Quotation:       "任意引文",
		ActorID:         actor,
	})
	if err != nil {
		t.Fatalf("无 chunk quotation citation 失败: %v", err)
	}
	if want := sha256Hex("任意引文"); noChunk.QuotationHash == nil || *noChunk.QuotationHash != want {
		t.Fatalf("quotation_hash = %v，期望 %q", noChunk.QuotationHash, want)
	}
}

func TestCreateCitation_Validation(t *testing.T) {
	tdb, svc, _ := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	src := createSource(t, svc, actor)
	ctx := context.Background()

	v1 := addVersion(t, svc, src.ID, "hash-v1", []evidence.ChunkInput{
		{Ordinal: 0, TextContent: "版本一的第一段。"},
	})
	v2 := addVersion(t, svc, src.ID, "hash-v2", []evidence.ChunkInput{
		{Ordinal: 0, TextContent: "版本二改写后的第一段。"},
	})
	chunkV1 := v1.Chunks[0]
	chunkV2 := v2.Chunks[0]

	create := func(params evidence.CreateCitationParams) error {
		_, err := svc.CreateCitation(ctx, params)
		return err
	}

	// source_version 不存在。
	if err := create(evidence.CreateCitationParams{
		SourceVersionID: tdb.NewID(t), ActorID: actor,
	}); !errors.Is(err, evidence.ErrSourceVersionNotFound) {
		t.Fatalf("version 不存在 err = %v，期望 ErrSourceVersionNotFound", err)
	}
	// chunk 不存在。
	missingChunk := tdb.NewID(t)
	if err := create(evidence.CreateCitationParams{
		SourceVersionID: v1.Version.ID, SourceChunkID: &missingChunk, ActorID: actor,
	}); !errors.Is(err, evidence.ErrSourceChunkNotFound) {
		t.Fatalf("chunk 不存在 err = %v，期望 ErrSourceChunkNotFound", err)
	}
	// chunk 跨版本拒绝。
	if err := create(evidence.CreateCitationParams{
		SourceVersionID: v1.Version.ID, SourceChunkID: &chunkV2.ID, ActorID: actor,
	}); !errors.Is(err, evidence.ErrChunkVersionMismatch) {
		t.Fatalf("跨版本 chunk err = %v，期望 ErrChunkVersionMismatch", err)
	}
	// quotation 不是 chunk 文本子串：严格拒绝。
	if err := create(evidence.CreateCitationParams{
		SourceVersionID: v1.Version.ID, SourceChunkID: &chunkV1.ID,
		Quotation: "版本二改写后的第一段。", ActorID: actor,
	}); !errors.Is(err, evidence.ErrQuotationMismatch) {
		t.Fatalf("quotation 非子串 err = %v，期望 ErrQuotationMismatch", err)
	}
	// locator 非法。
	badPage := int32(0)
	if err := create(evidence.CreateCitationParams{
		SourceVersionID: v1.Version.ID, Locator: &evidence.Locator{Page: &badPage}, ActorID: actor,
	}); !errors.Is(err, evidence.ErrInvalidLocator) {
		t.Fatalf("locator err = %v，期望 ErrInvalidLocator", err)
	}
	// Actor 准入：ai actor 拒绝。
	aiActor := tdb.MakeActor(t, "ai", "llm")
	if err := create(evidence.CreateCitationParams{
		SourceVersionID: v1.Version.ID, ActorID: aiActor,
	}); !errors.Is(err, page.ErrActorNotAllowed) {
		t.Fatalf("ai actor err = %v，期望 ErrActorNotAllowed", err)
	}
}
