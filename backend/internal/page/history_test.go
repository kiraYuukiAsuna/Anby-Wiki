// Revision 历史、结构 Diff 与回滚的领域层集成测试（M1-T07）。
package page_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/page"
)

// fixedAST 构造固定 Block ID 的合法 AST（Diff/回滚需要跨版本稳定 ID）。
// 每个元素是 {blockID, text} 对，生成顶层 paragraph 序列。
func fixedAST(t *testing.T, blocks ...[2]string) json.RawMessage {
	t.Helper()
	doc := map[string]any{
		"type":           "document",
		"schema_version": 1,
		"children":       []any{},
	}
	for _, b := range blocks {
		doc["children"] = append(doc["children"].([]any), map[string]any{
			"id":   b[0],
			"type": "paragraph",
			"content": []any{
				map[string]any{"type": "text", "text": b[1]},
			},
		})
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// newBlockID 生成一个合法 Block ID（UUIDv7）。
func newBlockID(t *testing.T) string {
	t.Helper()
	id, err := ast.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// publishThree 创建页面并依次发布三个版本，返回 (page, [v1, v2, v3])。
func publishThree(t *testing.T, svc *page.Service, actor uuid.UUID, title string, asts ...json.RawMessage) (*page.Page, []*page.Revision) {
	t.Helper()
	p := createMainPage(t, svc, title, actor)
	revs := make([]*page.Revision, 0, len(asts))
	var expected *uuid.UUID
	for i, raw := range asts {
		rev, err := svc.Publish(context.Background(), page.PublishParams{
			PageID:             p.ID,
			ActorID:            actor,
			ExpectedRevisionID: expected,
			AST:                raw,
			Summary:            fmt.Sprintf("v%d", i+1),
		})
		if err != nil {
			t.Fatalf("发布 v%d 失败: %v", i+1, err)
		}
		revs = append(revs, rev)
		expected = &rev.ID
	}
	return p, revs
}

func TestListRevisions_OrderAndPagination(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	p, revs := publishThree(t, svc, actor, "History Page",
		validAST(t, "第一版"), validAST(t, "第二版"), validAST(t, "第三版"))
	v1, v2, v3 := revs[0], revs[1], revs[2]

	// 默认分页：倒序 [v3, v2, v1]，无下一页。
	result, err := svc.ListRevisions(ctx, p.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("items 数 = %d, want 3", len(result.Items))
	}
	for i, want := range []*page.Revision{v3, v2, v1} {
		got := result.Items[i]
		if got.ID != want.ID {
			t.Fatalf("items[%d].ID = %s, want %s", i, got.ID, want.ID)
		}
		if got.ContentHash != want.ContentHash || got.SchemaVersion != ast.SchemaVersion {
			t.Fatalf("items[%d] hash/version 异常: %+v", i, got)
		}
		if got.Summary != want.Summary || got.Visibility != page.VisibilityPublic || got.CreatedAt.IsZero() {
			t.Fatalf("items[%d] 元信息异常: %+v", i, got)
		}
	}
	if result.NextCursor != nil {
		t.Fatalf("首页全量返回不应有 next_cursor: %v", *result.NextCursor)
	}

	// page_size=2 翻页取全：第一页 [v3, v2]，第二页 [v1]。
	page1, err := svc.ListRevisions(ctx, p.ID, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Items) != 2 || page1.Items[0].ID != v3.ID || page1.Items[1].ID != v2.ID {
		t.Fatalf("第一页异常: %+v", page1.Items)
	}
	if page1.NextCursor == nil {
		t.Fatal("第一页应有 next_cursor")
	}
	page2, err := svc.ListRevisions(ctx, p.ID, *page1.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 1 || page2.Items[0].ID != v1.ID {
		t.Fatalf("第二页异常: %+v", page2.Items)
	}
	if page2.NextCursor != nil {
		t.Fatalf("末页不应有 next_cursor: %v", *page2.NextCursor)
	}

	// 游标非法：ErrInvalidCursor。
	for _, bad := range []string{"not-base64!!!", "bm90LXNwbGl0", "MTIzOm5vdC11dWlk"} {
		if _, err := svc.ListRevisions(ctx, p.ID, bad, 2); !errors.Is(err, page.ErrInvalidCursor) {
			t.Fatalf("cursor=%q err = %v, want ErrInvalidCursor", bad, err)
		}
	}

	// 页面不存在：ErrPageNotFound。
	if _, err := svc.ListRevisions(ctx, uuid.New(), "", 0); !errors.Is(err, page.ErrPageNotFound) {
		t.Fatalf("err = %v, want ErrPageNotFound", err)
	}
}

func TestGetRevision_DetailAndCrossPage(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	p, revs := publishThree(t, svc, actor, "Detail Page", validAST(t, "一"), validAST(t, "二"), validAST(t, "三"))
	other, otherRevs := publishThree(t, svc, actor, "Other Page", validAST(t, "x"))

	// 单版详情：元信息 + AST 字节。
	rev, snap, err := svc.GetRevision(ctx, p.ID, revs[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if rev.ID != revs[1].ID || rev.ParentRevisionID == nil || *rev.ParentRevisionID != revs[0].ID {
		t.Fatalf("revision 异常: %+v", rev)
	}
	canonical, err := ast.CanonicalizeJSON(snap.AST)
	if err != nil {
		t.Fatal(err)
	}
	if rev.ContentHash != revs[1].ContentHash || len(canonical) == 0 {
		t.Fatalf("快照异常: hash=%s", rev.ContentHash)
	}
	var doc map[string]any
	if err := json.Unmarshal(snap.AST, &doc); err != nil || doc["type"] != "document" {
		t.Fatalf("AST 非合法 document: %v", err)
	}

	// 跨页访问：不得泄露，返回 ErrRevisionNotFound。
	if _, _, err := svc.GetRevision(ctx, p.ID, otherRevs[0].ID); !errors.Is(err, page.ErrRevisionNotFound) {
		t.Fatalf("err = %v, want ErrRevisionNotFound", err)
	}
	if _, _, err := svc.GetRevision(ctx, other.ID, revs[0].ID); !errors.Is(err, page.ErrRevisionNotFound) {
		t.Fatalf("err = %v, want ErrRevisionNotFound", err)
	}
	// 页面不存在：ErrPageNotFound。
	if _, _, err := svc.GetRevision(ctx, uuid.New(), revs[0].ID); !errors.Is(err, page.ErrPageNotFound) {
		t.Fatalf("err = %v, want ErrPageNotFound", err)
	}
	// Revision 不存在：ErrRevisionNotFound。
	if _, _, err := svc.GetRevision(ctx, p.ID, uuid.New()); !errors.Is(err, page.ErrRevisionNotFound) {
		t.Fatalf("err = %v, want ErrRevisionNotFound", err)
	}
}

func TestDiffRevisions(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	blockA, blockB, blockC := newBlockID(t), newBlockID(t), newBlockID(t)

	astV1 := fixedAST(t, [2]string{blockA, "alpha"}, [2]string{blockB, "beta"})
	astV2 := fixedAST(t, [2]string{blockA, "alpha"}, [2]string{blockB, "beta"})
	astV3 := fixedAST(t, [2]string{blockA, "alpha 改"}, [2]string{blockC, "gamma"})
	p, revs := publishThree(t, svc, actor, "Diff Page", astV1, astV2, astV3)
	v1, v3 := revs[0], revs[2]

	// from=v1, to=v3：A changed（content 字段）、C added、B removed。
	diff, err := svc.DiffRevisions(ctx, p.ID, v1.ID, v3.ID)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string][]ast.ChangeType{}
	var changedFields []ast.FieldChange
	for _, ch := range diff.Changes {
		byID[ch.BlockID] = append(byID[ch.BlockID], ch.Type)
		if ch.Type == ast.ChangeChanged {
			changedFields = ch.Fields
		}
	}
	if got := byID[blockA]; len(got) != 1 || got[0] != ast.ChangeChanged {
		t.Fatalf("blockA 变更 = %v, want [changed]", got)
	}
	if got := byID[blockB]; len(got) != 1 || got[0] != ast.ChangeRemoved {
		t.Fatalf("blockB 变更 = %v, want [removed]", got)
	}
	if got := byID[blockC]; len(got) != 1 || got[0] != ast.ChangeAdded {
		t.Fatalf("blockC 变更 = %v, want [added]", got)
	}
	if len(changedFields) == 0 {
		t.Fatal("changed 条目应带字段级 before/after")
	}
	foundContent := false
	for _, f := range changedFields {
		if f.Field == "content" {
			foundContent = true
		}
	}
	if !foundContent {
		t.Fatalf("changed 字段应含 content: %+v", changedFields)
	}

	// 同版 Diff 为空。
	diff, err = svc.DiffRevisions(ctx, p.ID, v1.ID, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Changes) != 0 {
		t.Fatalf("同版 Diff 应为空, 得到 %+v", diff.Changes)
	}

	// 反向 Diff（v3→v1）：B added、C removed、A changed。
	diff, err = svc.DiffRevisions(ctx, p.ID, v3.ID, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	byID = map[string][]ast.ChangeType{}
	for _, ch := range diff.Changes {
		byID[ch.BlockID] = append(byID[ch.BlockID], ch.Type)
	}
	if byID[blockB][0] != ast.ChangeAdded || byID[blockC][0] != ast.ChangeRemoved || byID[blockA][0] != ast.ChangeChanged {
		t.Fatalf("反向 Diff 异常: %+v", diff.Changes)
	}

	// 任一 Revision 不属于本页：ErrRevisionNotFound。
	_, otherRevs := publishThree(t, svc, actor, "Diff Other", validAST(t, "x"))
	if _, err := svc.DiffRevisions(ctx, p.ID, v1.ID, otherRevs[0].ID); !errors.Is(err, page.ErrRevisionNotFound) {
		t.Fatalf("err = %v, want ErrRevisionNotFound", err)
	}
}

func TestRollback(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	blockA := newBlockID(t)
	astV1 := fixedAST(t, [2]string{blockA, "最初内容"})
	p, revs := publishThree(t, svc, actor, "Rollback Page",
		astV1, validAST(t, "第二版"), validAST(t, "第三版"))
	v1, v3 := revs[0], revs[2]

	// 回滚前记录 v1 快照/Revision 状态，供事后断言未改动。
	beforeSnapCount := countRows(t, tdb, "content_snapshot", "")
	v1RevBefore, v1SnapBefore, err := svc.GetRevision(ctx, p.ID, v1.ID)
	if err != nil {
		t.Fatal(err)
	}

	// ---- 回滚到 v1：产生 v4 ----
	v4, err := svc.Rollback(ctx, page.RollbackParams{PageID: p.ID, TargetRevisionID: v1.ID, ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	if v4.ParentRevisionID == nil || *v4.ParentRevisionID != v3.ID {
		t.Fatalf("v4 parent = %v, want %s", v4.ParentRevisionID, v3.ID)
	}
	if v4.ID == v1.ID {
		t.Fatal("回滚必须新建 Revision")
	}
	if v4.ContentHash != v1.ContentHash {
		t.Fatalf("v4 content_hash = %s, want 与 v1 相同 %s", v4.ContentHash, v1.ContentHash)
	}
	// 决策：内容 hash 相同则复用已有 content_snapshot 行，不重复存储。
	if v4.ContentSnapshotID != v1.ContentSnapshotID {
		t.Fatalf("v4 snapshot = %s, want 复用 v1 的 %s", v4.ContentSnapshotID, v1.ContentSnapshotID)
	}
	if n := countRows(t, tdb, "content_snapshot", ""); n != beforeSnapCount {
		t.Fatalf("回滚后 snapshot 数 = %d, want %d（去重复用）", n, beforeSnapCount)
	}
	if v4.Summary != "回滚到 "+v1.ID.String() {
		t.Fatalf("默认 summary = %q", v4.Summary)
	}

	// current 指向 v4。
	got, err := svc.GetPage(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentRevisionID == nil || *got.CurrentRevisionID != v4.ID {
		t.Fatalf("current = %v, want %s", got.CurrentRevisionID, v4.ID)
	}

	// v4 内容与 v1 的 canonical AST 相同。
	_, v4Snap, err := svc.GetRevision(ctx, p.ID, v4.ID)
	if err != nil {
		t.Fatal(err)
	}
	canonV1, _ := ast.CanonicalizeJSON(astV1)
	canonV4, err := ast.CanonicalizeJSON(v4Snap.AST)
	if err != nil {
		t.Fatal(err)
	}
	if string(canonV4) != string(canonV1) {
		t.Fatalf("v4 AST = %s, want 与 v1 相同 %s", canonV4, canonV1)
	}

	// 审计：revision.rolled_back，payload 含 rolled_back_to。
	var auditPayload []byte
	err = tdb.Pool.QueryRow(ctx, `
		SELECT payload_json::text FROM audit_event
		WHERE aggregate_id = $1 AND event_type = 'revision.rolled_back'`, p.ID,
	).Scan(&auditPayload)
	if err != nil {
		t.Fatalf("查询 rolled_back 审计失败: %v", err)
	}
	var ap map[string]any
	if err := json.Unmarshal(auditPayload, &ap); err != nil {
		t.Fatal(err)
	}
	if ap["rolled_back_to"] != v1.ID.String() || ap["revision_id"] != v4.ID.String() {
		t.Fatalf("rollback 审计 payload 异常: %s", auditPayload)
	}
	// Outbox：回滚同样驱动投影重建（page.revision_published）。
	if n := countRows(t, tdb, "outbox_event", "aggregate_id = $1 AND event_type = 'page.revision_published'", p.ID); n != 4 {
		t.Fatalf("outbox 数 = %d, want 4（3 发布 + 1 回滚）", n)
	}

	// INV-02：v1 的 Revision 与 Snapshot 未被改动。
	v1RevAfter, v1SnapAfter, err := svc.GetRevision(ctx, p.ID, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *v1RevAfter != *v1RevBefore {
		t.Fatalf("v1 revision 被改动: before=%+v after=%+v", v1RevBefore, v1RevAfter)
	}
	canonBefore, _ := ast.CanonicalizeJSON(v1SnapBefore.AST)
	canonAfter, _ := ast.CanonicalizeJSON(v1SnapAfter.AST)
	if string(canonBefore) != string(canonAfter) || v1SnapAfter.ID != v1SnapBefore.ID {
		t.Fatal("v1 snapshot 被改动")
	}

	// 调用方覆盖 summary。
	v5, err := svc.Rollback(ctx, page.RollbackParams{
		PageID: p.ID, TargetRevisionID: v1.ID, ActorID: actor, Summary: "撤销破坏",
	})
	if err != nil {
		t.Fatal(err)
	}
	if v5.Summary != "撤销破坏" || v5.ParentRevisionID == nil || *v5.ParentRevisionID != v4.ID {
		t.Fatalf("v5 异常: %+v", v5)
	}

	// 目标 Revision 不属于本页：ErrRevisionNotFound。
	_, otherRevs := publishThree(t, svc, actor, "Rollback Other", validAST(t, "x"))
	_, err = svc.Rollback(ctx, page.RollbackParams{PageID: p.ID, TargetRevisionID: otherRevs[0].ID, ActorID: actor})
	if !errors.Is(err, page.ErrRevisionNotFound) {
		t.Fatalf("err = %v, want ErrRevisionNotFound", err)
	}
	// 页面不存在：ErrPageNotFound。
	_, err = svc.Rollback(ctx, page.RollbackParams{PageID: uuid.New(), TargetRevisionID: v1.ID, ActorID: actor})
	if !errors.Is(err, page.ErrPageNotFound) {
		t.Fatalf("err = %v, want ErrPageNotFound", err)
	}
	// ai actor：ErrActorNotAllowed。
	ai := tdb.MakeActor(t, "ai", "gpt")
	_, err = svc.Rollback(ctx, page.RollbackParams{PageID: p.ID, TargetRevisionID: v1.ID, ActorID: ai})
	if !errors.Is(err, page.ErrActorNotAllowed) {
		t.Fatalf("err = %v, want ErrActorNotAllowed", err)
	}

	// 回滚之后再用旧基线发布：ErrStaleRevision（陈旧基线拒绝）。
	_, err = svc.Publish(ctx, page.PublishParams{
		PageID: p.ID, ActorID: actor, ExpectedRevisionID: &v3.ID, AST: validAST(t, "陈旧"),
	})
	if !errors.Is(err, page.ErrStaleRevision) {
		t.Fatalf("err = %v, want ErrStaleRevision", err)
	}
}

func TestRollback_ConcurrentWithPublish(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	p, revs := publishThree(t, svc, actor, "Concurrent Rollback",
		validAST(t, "一"), validAST(t, "二"), validAST(t, "三"))
	v1, v3 := revs[0], revs[2]

	// barrier 同时放行：回滚到 v1 vs 以 v3 为基线的发布。
	// 行锁串行化：发布若落后则基线过期被拒；回滚以锁内 current 为基线，始终追加成功。
	start := make(chan struct{})
	var wg sync.WaitGroup
	var rollbackRev *page.Revision
	var rollbackErr, publishErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		rollbackRev, rollbackErr = svc.Rollback(ctx, page.RollbackParams{
			PageID: p.ID, TargetRevisionID: v1.ID, ActorID: actor,
		})
	}()
	go func() {
		defer wg.Done()
		<-start
		_, publishErr = svc.Publish(ctx, page.PublishParams{
			PageID: p.ID, ActorID: actor, ExpectedRevisionID: &v3.ID, AST: validAST(t, "并发第四版"),
		})
	}()
	close(start)
	wg.Wait()

	if rollbackErr != nil {
		t.Fatalf("回滚不应失败: %v", rollbackErr)
	}
	publishSucceeded := publishErr == nil
	if !publishSucceeded && !errors.Is(publishErr, page.ErrStaleRevision) {
		t.Fatalf("发布失败应为 ErrStaleRevision, 得到 %v", publishErr)
	}

	// 状态一致：revision 数 = 3 + 回滚 1 +（发布成功 ? 1 : 0），无双写。
	want := 4
	if publishSucceeded {
		want = 5
	}
	if n := countRows(t, tdb, "revision", "page_id = $1", p.ID); n != want {
		t.Fatalf("revision 数 = %d, want %d", n, want)
	}
	// current 是版本链头：回滚胜者或发布胜者，均无悬空。
	got, err := svc.GetPage(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentRevisionID == nil {
		t.Fatal("current 不应为空")
	}
	if _, _, err := svc.GetRevision(ctx, p.ID, *got.CurrentRevisionID); err != nil {
		t.Fatalf("current 指向的 revision 不可读: %v", err)
	}
	if rollbackRev.ParentRevisionID == nil {
		t.Fatal("回滚 parent 不应为空")
	}
	if !publishSucceeded && *rollbackRev.ParentRevisionID != v3.ID {
		// 发布被拒 ⇒ 回滚先拿到行锁，parent 必为 v3。
		t.Fatalf("回滚 parent = %v, want %s", rollbackRev.ParentRevisionID, v3.ID)
	}
}
