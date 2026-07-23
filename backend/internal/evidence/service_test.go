package evidence_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/platform/storage"
	"github.com/anby/wiki/backend/testkit"
)

// testEnv 对象键的环境段（ADR-0004 键约定的第一段）。
const testEnv = "test"

// setup 建连 + Reset，并装配领域服务（对象存储用内存 Fake）。
func setup(t *testing.T) (*testkit.DB, *evidence.Service, *storage.Fake) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	fake := storage.NewFake()
	svc := evidence.NewService(
		evidence.NewRepository(tdb.Pool),
		page.NewRepository(tdb.Pool),
		fake,
		testEnv,
		db.NewTxManager(tdb.Pool),
		id.NewGenerator(),
	)
	return tdb, svc, fake
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestStoreAsset_Basic(t *testing.T) {
	tdb, svc, fake := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")

	content := "hello evidence"
	res, err := svc.StoreAsset(context.Background(), evidence.StoreAssetParams{
		WikiID:   testkit.DefaultWikiID,
		Name:     "anby.png",
		Content:  strings.NewReader(content),
		MimeType: "image/png",
		ActorID:  actor,
	})
	if err != nil {
		t.Fatalf("StoreAsset 失败: %v", err)
	}
	if res.Reused {
		t.Fatalf("首次上传 Reused = true，期望 false")
	}
	if res.Asset.Name != "anby.png" || res.Asset.Status != evidence.AssetStatusActive {
		t.Fatalf("asset = %+v，字段不符合预期", res.Asset)
	}
	if res.Asset.CurrentRevisionID == nil || *res.Asset.CurrentRevisionID != res.Revision.ID {
		t.Fatalf("current_revision_id = %v，期望指向 %s", res.Asset.CurrentRevisionID, res.Revision.ID)
	}

	wantHash := sha256Hex(content)
	wantKey := testEnv + "/asset/" + wantHash[:2] + "/" + wantHash
	rev := res.Revision
	if rev.ContentHash != wantHash {
		t.Fatalf("content_hash = %q，期望 %q", rev.ContentHash, wantHash)
	}
	if rev.StorageKey != wantKey {
		t.Fatalf("storage_key = %q，期望 %q", rev.StorageKey, wantKey)
	}
	if rev.SizeBytes != int64(len(content)) {
		t.Fatalf("size_bytes = %d，期望 %d", rev.SizeBytes, len(content))
	}
	if rev.MimeType != "image/png" || rev.ActorID != actor {
		t.Fatalf("revision = %+v，mime_type/actor_id 不符合预期", rev)
	}

	// 对象存储按内容寻址键落盘。
	meta, err := fake.Head(context.Background(), wantKey)
	if err != nil {
		t.Fatalf("fake Head(%q) 失败: %v", wantKey, err)
	}
	if meta.Size != int64(len(content)) || meta.ContentType != "image/png" {
		t.Fatalf("对象元数据 = %+v，不符合预期", meta)
	}
	if keys := fake.Keys(); len(keys) != 1 || keys[0] != wantKey {
		t.Fatalf("fake keys = %v，期望 [%q]", keys, wantKey)
	}
}

func TestStoreAsset_DedupSameContent(t *testing.T) {
	tdb, svc, fake := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")

	params := evidence.StoreAssetParams{
		WikiID:   testkit.DefaultWikiID,
		Name:     "anby.png",
		Content:  strings.NewReader("same content"),
		MimeType: "image/png",
		ActorID:  actor,
	}
	first, err := svc.StoreAsset(context.Background(), params)
	if err != nil {
		t.Fatalf("首次 StoreAsset 失败: %v", err)
	}
	params.Content = strings.NewReader("same content")
	second, err := svc.StoreAsset(context.Background(), params)
	if err != nil {
		t.Fatalf("重复 StoreAsset 失败: %v", err)
	}

	if !second.Reused {
		t.Fatalf("重复上传 Reused = false，期望 true")
	}
	if second.Asset.ID != first.Asset.ID {
		t.Fatalf("asset id = %s，期望复用 %s", second.Asset.ID, first.Asset.ID)
	}
	if second.Revision.ID != first.Revision.ID {
		t.Fatalf("revision id = %s，期望复用 %s", second.Revision.ID, first.Revision.ID)
	}
	if got := fake.PutCount(); got != 1 {
		t.Fatalf("PutCount = %d，期望 1（去重不重复 Put）", got)
	}
}

func TestStoreAsset_NewContentNewRevision(t *testing.T) {
	tdb, svc, fake := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")

	first, err := svc.StoreAsset(context.Background(), evidence.StoreAssetParams{
		WikiID:   testkit.DefaultWikiID,
		Name:     "anby.png",
		Content:  strings.NewReader("v1 content"),
		MimeType: "image/png",
		ActorID:  actor,
	})
	if err != nil {
		t.Fatalf("v1 StoreAsset 失败: %v", err)
	}
	second, err := svc.StoreAsset(context.Background(), evidence.StoreAssetParams{
		WikiID:   testkit.DefaultWikiID,
		Name:     "anby.png",
		Content:  strings.NewReader("v2 content changed"),
		MimeType: "image/png",
		ActorID:  actor,
	})
	if err != nil {
		t.Fatalf("v2 StoreAsset 失败: %v", err)
	}

	if second.Reused {
		t.Fatalf("新内容 Reused = true，期望 false")
	}
	if second.Asset.ID != first.Asset.ID {
		t.Fatalf("同名新内容应复用 asset %s，实际 %s", first.Asset.ID, second.Asset.ID)
	}
	if second.Revision.ID == first.Revision.ID {
		t.Fatalf("新内容应新增 asset_revision，实际复用 %s", second.Revision.ID)
	}
	if second.Asset.CurrentRevisionID == nil || *second.Asset.CurrentRevisionID != second.Revision.ID {
		t.Fatalf("current_revision_id 未指向新版本")
	}
	if got := fake.PutCount(); got != 2 {
		t.Fatalf("PutCount = %d，期望 2", got)
	}

	// 再传一次 v1 内容：去重命中旧 revision，current 指回 v1。
	third, err := svc.StoreAsset(context.Background(), evidence.StoreAssetParams{
		WikiID:   testkit.DefaultWikiID,
		Name:     "anby.png",
		Content:  strings.NewReader("v1 content"),
		MimeType: "image/png",
		ActorID:  actor,
	})
	if err != nil {
		t.Fatalf("v1 重传失败: %v", err)
	}
	if !third.Reused || third.Revision.ID != first.Revision.ID {
		t.Fatalf("v1 重传应去重命中首个 revision %s，实际 %+v", first.Revision.ID, third)
	}
	if *third.Asset.CurrentRevisionID != first.Revision.ID {
		t.Fatalf("v1 重传后 current 应指回 %s", first.Revision.ID)
	}
	if got := fake.PutCount(); got != 2 {
		t.Fatalf("v1 重传后 PutCount = %d，期望 2（不重复 Put）", got)
	}
}

func TestStoreAsset_InvalidInput(t *testing.T) {
	tdb, svc, _ := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")

	cases := []struct {
		name   string
		params evidence.StoreAssetParams
	}{
		{"空名字", evidence.StoreAssetParams{WikiID: testkit.DefaultWikiID, Name: "  ", Content: strings.NewReader("x"), MimeType: "image/png", ActorID: actor}},
		{"空 mime", evidence.StoreAssetParams{WikiID: testkit.DefaultWikiID, Name: "a.png", Content: strings.NewReader("x"), MimeType: "", ActorID: actor}},
		{"nil content", evidence.StoreAssetParams{WikiID: testkit.DefaultWikiID, Name: "a.png", Content: nil, MimeType: "image/png", ActorID: actor}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.StoreAsset(context.Background(), tc.params)
			if !errors.Is(err, evidence.ErrInvalidAssetInput) {
				t.Fatalf("err = %v，期望 ErrInvalidAssetInput", err)
			}
		})
	}
}

func TestStoreAsset_InvalidActor(t *testing.T) {
	_, svc, _ := setup(t)

	// 不存在的 actor。
	if _, err := svc.StoreAsset(context.Background(), evidence.StoreAssetParams{
		WikiID:   testkit.DefaultWikiID,
		Name:     "a.png",
		Content:  strings.NewReader("x"),
		MimeType: "image/png",
		ActorID:  uuid.MustParse("00000000-0000-7000-8000-999999999999"),
	}); !errors.Is(err, page.ErrInvalidActor) {
		t.Fatalf("不存在 actor err = %v，期望 ErrInvalidActor", err)
	}
}

// TestAssetRevisionImmutable SQL 级：asset_revision 拒绝 UPDATE/DELETE（000007 触发器）。
func TestAssetRevisionImmutable(t *testing.T) {
	tdb, svc, _ := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	res, err := svc.StoreAsset(context.Background(), evidence.StoreAssetParams{
		WikiID:   testkit.DefaultWikiID,
		Name:     "anby.png",
		Content:  strings.NewReader("immutable"),
		MimeType: "image/png",
		ActorID:  actor,
	})
	if err != nil {
		t.Fatalf("StoreAsset 失败: %v", err)
	}

	ctx := context.Background()
	if _, err := tdb.Pool.Exec(ctx, `
		UPDATE asset_revision SET mime_type = 'text/plain' WHERE id = $1`, res.Revision.ID); err == nil ||
		!strings.Contains(err.Error(), "不允许 UPDATE") {
		t.Fatalf("UPDATE asset_revision err = %v，期望不可变拒绝", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `
		DELETE FROM asset_revision WHERE id = $1`, res.Revision.ID); err == nil ||
		!strings.Contains(err.Error(), "不允许 DELETE") {
		t.Fatalf("DELETE asset_revision err = %v，期望不可变拒绝", err)
	}
}

// makeSourceChain SQL 级直插 source → source_version → source_chunk → citation，
// 返回四级行 ID（source/citation 的领域服务属 M4-T05，此处只为验证 000007 约束）。
func makeSourceChain(t *testing.T, tdb *testkit.DB, actor uuid.UUID) (sourceID, versionID, chunkID, citationID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	sourceID = tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO source (id, source_type, title) VALUES ($1, 'webpage', '示例来源')`, sourceID); err != nil {
		t.Fatalf("插入 source 失败: %v", err)
	}
	versionID = tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO source_version (id, source_id, version_hash, fetched_at)
		VALUES ($1, $2, 'v1hash', now())`, versionID, sourceID); err != nil {
		t.Fatalf("插入 source_version 失败: %v", err)
	}
	chunkID = tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO source_chunk (id, source_version_id, ordinal, text_content, text_hash)
		VALUES ($1, $2, 0, '正文片段', 'texthash')`, chunkID, versionID); err != nil {
		t.Fatalf("插入 source_chunk 失败: %v", err)
	}
	citationID = tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO citation (id, source_version_id, source_chunk_id, quotation, quotation_hash, created_by)
		VALUES ($1, $2, $3, '引用原文', 'quotehash', $4)`, citationID, versionID, chunkID, actor); err != nil {
		t.Fatalf("插入 citation 失败: %v", err)
	}
	return sourceID, versionID, chunkID, citationID
}

// TestEvidenceImmutableTables SQL 级：source_version / source_chunk / citation
// 拒绝 UPDATE/DELETE（000007 触发器）。
func TestEvidenceImmutableTables(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	actor := tdb.MakeActor(t, "human", "alice")
	_, versionID, chunkID, citationID := makeSourceChain(t, tdb, actor)

	ctx := context.Background()
	cases := []struct {
		table string
		id    uuid.UUID
	}{
		{"source_version", versionID},
		{"source_chunk", chunkID},
		{"citation", citationID},
	}
	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			if _, err := tdb.Pool.Exec(ctx, `
				UPDATE `+tc.table+` SET created_at = now() WHERE id = $1`, tc.id); err == nil ||
				!strings.Contains(err.Error(), "不允许 UPDATE") {
				t.Fatalf("UPDATE %s err = %v，期望不可变拒绝", tc.table, err)
			}
			if _, err := tdb.Pool.Exec(ctx, `
				DELETE FROM `+tc.table+` WHERE id = $1`, tc.id); err == nil ||
				!strings.Contains(err.Error(), "不允许 DELETE") {
				t.Fatalf("DELETE %s err = %v，期望不可变拒绝", tc.table, err)
			}
		})
	}
}

// TestClaimSourceCitationFK SQL 级：claim_source.citation_id 外键（000007 补）
// 拒绝不存在的 citation；合法引用可正常插入。
func TestClaimSourceCitationFK(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	// 搭一个最小 claim（entity 用种子 person 类型直插）。
	entityID := tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO entity (id, wiki_id, entity_type_id, canonical_key, created_by)
		VALUES ($1, $2, $3, 'anby-demara', $4)`,
		entityID, testkit.DefaultWikiID, testkit.EntityTypePersonID, actor); err != nil {
		t.Fatalf("插入 entity 失败: %v", err)
	}
	claimID := tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO claim (id, subject_entity_id, property_id, value_type, origin_type, created_by)
		VALUES ($1, $2, $3, 'entity', 'human', $4)`,
		claimID, entityID, testkit.PropertyInstanceOfID, actor); err != nil {
		t.Fatalf("插入 claim 失败: %v", err)
	}

	// 不存在的 citation → FK 拒绝。
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO claim_source (claim_id, citation_id, support_type)
		VALUES ($1, $2, 'supports')`, claimID, tdb.NewID(t)); err == nil ||
		!strings.Contains(err.Error(), "claim_source_citation_fk") {
		t.Fatalf("claim_source 插不存在 citation err = %v，期望 claim_source_citation_fk 外键拒绝", err)
	}

	// 合法 citation → 插入成功。
	_, _, _, citationID := makeSourceChain(t, tdb, actor)
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO claim_source (claim_id, citation_id, support_type)
		VALUES ($1, $2, 'supports')`, claimID, citationID); err != nil {
		t.Fatalf("claim_source 插合法 citation 失败: %v", err)
	}
}
