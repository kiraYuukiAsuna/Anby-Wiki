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
	"github.com/anby/wiki/backend/testkit"
)

// validAST 构造一个合法 AST v1 文档（单个 paragraph，文本由 text 指定）。
// block ID 每次随机生成，避免误依赖固定值。
func validAST(t *testing.T, text string) json.RawMessage {
	t.Helper()
	blockID, err := ast.NewID()
	if err != nil {
		t.Fatalf("生成 Block ID 失败: %v", err)
	}
	return json.RawMessage(fmt.Sprintf(
		`{"type":"document","schema_version":1,"children":[{"id":%q,"type":"paragraph","content":[{"type":"text","text":%q}]}]}`,
		blockID, text))
}

func publishOne(t *testing.T, svc *page.Service, pageID, actorID uuid.UUID, expected *uuid.UUID, astJSON json.RawMessage) *page.Revision {
	t.Helper()
	rev, err := svc.Publish(context.Background(), page.PublishParams{
		PageID:             pageID,
		ActorID:            actorID,
		ExpectedRevisionID: expected,
		AST:                astJSON,
		Summary:            "test publish",
	})
	if err != nil {
		t.Fatalf("Publish 失败: %v", err)
	}
	return rev
}

// countRows 统计表行数（where 可为空）。
func countRows(t *testing.T, tdb *testkit.DB, table, where string, args ...any) int {
	t.Helper()
	var n int
	q := "SELECT count(*) FROM " + table
	if where != "" {
		q += " WHERE " + where
	}
	if err := tdb.Pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("count %s 失败: %v", table, err)
	}
	return n
}

func TestPublish_FirstThenSecond(t *testing.T) {
	tdb, svc := setup(t)
	ctx := context.Background()
	actor := tdb.MakeActor(t, "human", "alice")
	p := createMainPage(t, svc, "Publish Target", actor)

	// ---- 首发布：expected 必须为 nil ----
	ast1 := validAST(t, "第一版")
	rev1 := publishOne(t, svc, p.ID, actor, nil, ast1)

	if rev1.ParentRevisionID != nil {
		t.Fatalf("首发布 parent 应为 nil, 得到 %v", rev1.ParentRevisionID)
	}
	if rev1.Visibility != page.VisibilityPublic || rev1.ActorID != actor || rev1.Summary != "test publish" {
		t.Fatalf("revision 字段异常: %+v", rev1)
	}
	if rev1.CreatedAt.IsZero() {
		t.Fatal("created_at 未回填")
	}

	// current 指针移动
	got, err := svc.GetPage(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentRevisionID == nil || *got.CurrentRevisionID != rev1.ID {
		t.Fatalf("current_revision_id = %v, want %s", got.CurrentRevisionID, rev1.ID)
	}

	// snapshot：canonical 字节、hash 与 size 由服务端计算且互相一致
	var (
		storedAST     []byte
		storedHash    string
		storedSize    int
		schemaVersion int
	)
	err = tdb.Pool.QueryRow(ctx, `
		SELECT ast_json::text, content_hash, size_bytes, schema_version
		FROM content_snapshot WHERE id = $1`, rev1.ContentSnapshotID,
	).Scan(&storedAST, &storedHash, &storedSize, &schemaVersion)
	if err != nil {
		t.Fatalf("查询 snapshot 失败: %v", err)
	}
	canonical, err := ast.CanonicalizeJSON(ast1)
	if err != nil {
		t.Fatal(err)
	}
	// jsonb 存储会重排键序/空白，取回后重新 canonicalize 再比较语义等价性
	storedCanonical, err := ast.CanonicalizeJSON(storedAST)
	if err != nil {
		t.Fatal(err)
	}
	if string(storedCanonical) != string(canonical) {
		t.Fatalf("snapshot AST = %s, want canonical %s", storedCanonical, canonical)
	}
	if storedHash != rev1.ContentHash || storedSize != len(canonical) || schemaVersion != ast.SchemaVersion {
		t.Fatalf("snapshot hash/size/version 异常: hash=%s size=%d version=%d", storedHash, storedSize, schemaVersion)
	}

	// audit + outbox 各一行，payload 字段齐全
	assertPublishEvents(t, tdb, p.ID, rev1)

	// ---- 二次发布：parent 链与 current 移动 ----
	rev2 := publishOne(t, svc, p.ID, actor, &rev1.ID, validAST(t, "第二版"))
	if rev2.ParentRevisionID == nil || *rev2.ParentRevisionID != rev1.ID {
		t.Fatalf("rev2 parent = %v, want %s", rev2.ParentRevisionID, rev1.ID)
	}
	got, err = svc.GetPage(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *got.CurrentRevisionID != rev2.ID {
		t.Fatalf("current = %v, want %s", got.CurrentRevisionID, rev2.ID)
	}
	if n := countRows(t, tdb, "revision", "page_id = $1", p.ID); n != 2 {
		t.Fatalf("revision 数 = %d, want 2", n)
	}

	// ---- 陈旧基线：expected=rev1（current 已是 rev2）----
	_, err = svc.Publish(ctx, page.PublishParams{
		PageID: p.ID, ActorID: actor, ExpectedRevisionID: &rev1.ID, AST: validAST(t, "第三版"),
	})
	if !errors.Is(err, page.ErrStaleRevision) {
		t.Fatalf("err = %v, want ErrStaleRevision", err)
	}
	// 首发布语义反转：已发布页面传 nil expected 同样过期
	_, err = svc.Publish(ctx, page.PublishParams{
		PageID: p.ID, ActorID: actor, AST: validAST(t, "第三版"),
	})
	if !errors.Is(err, page.ErrStaleRevision) {
		t.Fatalf("err = %v, want ErrStaleRevision", err)
	}
	// 未发布页面传非 nil expected 也是过期
	q := createMainPage(t, svc, "Fresh Page", actor)
	someID := tdb.NewID(t)
	_, err = svc.Publish(ctx, page.PublishParams{
		PageID: q.ID, ActorID: actor, ExpectedRevisionID: &someID, AST: validAST(t, "x"),
	})
	if !errors.Is(err, page.ErrStaleRevision) {
		t.Fatalf("err = %v, want ErrStaleRevision", err)
	}
	// 过期发布不得留下任何残留
	if n := countRows(t, tdb, "revision", "page_id = $1", p.ID); n != 2 {
		t.Fatalf("过期发布后 revision 数 = %d, want 2", n)
	}
	if n := countRows(t, tdb, "revision", "page_id = $1", q.ID); n != 0 {
		t.Fatalf("fresh page revision 数 = %d, want 0", n)
	}
}

// assertPublishEvents 断言 rev 对应的 audit_event 与 outbox_event 各一行且字段齐全。
func assertPublishEvents(t *testing.T, tdb *testkit.DB, pageID uuid.UUID, rev *page.Revision) {
	t.Helper()
	ctx := context.Background()

	var (
		auditType    string
		auditAggType string
		auditPayload []byte
	)
	err := tdb.Pool.QueryRow(ctx, `
		SELECT event_type, aggregate_type, payload_json::text FROM audit_event
		WHERE aggregate_id = $1 AND event_type = 'revision.published'`, pageID,
	).Scan(&auditType, &auditAggType, &auditPayload)
	if err != nil {
		t.Fatalf("查询 audit_event 失败: %v", err)
	}
	if auditAggType != "page" {
		t.Fatalf("audit aggregate_type = %q", auditAggType)
	}
	var ap map[string]any
	if err := json.Unmarshal(auditPayload, &ap); err != nil {
		t.Fatal(err)
	}
	if ap["page_id"] != pageID.String() || ap["revision_id"] != rev.ID.String() ||
		ap["content_hash"] != rev.ContentHash {
		t.Fatalf("audit payload 异常: %s", auditPayload)
	}
	if _, ok := ap["parent_revision_id"]; !ok {
		t.Fatal("audit payload 缺少 parent_revision_id")
	}
	if rev.ParentRevisionID == nil && ap["parent_revision_id"] != nil {
		t.Fatalf("首发布 audit parent 应为 null: %s", auditPayload)
	}

	var (
		outboxType    string
		outboxStatus  string
		outboxPayload []byte
	)
	err = tdb.Pool.QueryRow(ctx, `
		SELECT event_type, status, payload_json::text FROM outbox_event
		WHERE aggregate_id = $1 AND event_type = 'page.revision_published'`, pageID,
	).Scan(&outboxType, &outboxStatus, &outboxPayload)
	if err != nil {
		t.Fatalf("查询 outbox_event 失败: %v", err)
	}
	if outboxStatus != "pending" {
		t.Fatalf("outbox status = %q, want pending", outboxStatus)
	}
	var op map[string]any
	if err := json.Unmarshal(outboxPayload, &op); err != nil {
		t.Fatal(err)
	}
	if op["page_id"] != pageID.String() || op["revision_id"] != rev.ID.String() ||
		op["content_hash"] != rev.ContentHash || op["schema_version"] != float64(ast.SchemaVersion) {
		t.Fatalf("outbox payload 异常: %s", outboxPayload)
	}
}

func TestPublish_ConcurrentDoublePublish(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	p := createMainPage(t, svc, "Concurrent Target", actor)
	rev1 := publishOne(t, svc, p.ID, actor, nil, validAST(t, "base"))

	// barrier 同时放行两个以相同 expected 发布的 goroutine
	start := make(chan struct{})
	results := make([]error, 2)
	revs := make([]*page.Revision, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			rev, err := svc.Publish(context.Background(), page.PublishParams{
				PageID:             p.ID,
				ActorID:            actor,
				ExpectedRevisionID: &rev1.ID,
				AST:                validAST(t, fmt.Sprintf("并发版本 %d", i)),
				Summary:            fmt.Sprintf("concurrent %d", i),
			})
			revs[i], results[i] = rev, err
		}(i)
	}
	close(start)
	wg.Wait()

	// 恰一个成功，另一个 ErrStaleRevision
	var succeeded, stale int
	var winner *page.Revision
	for i := 0; i < 2; i++ {
		switch {
		case results[i] == nil:
			succeeded++
			winner = revs[i]
		case errors.Is(results[i], page.ErrStaleRevision):
			stale++
		default:
			t.Fatalf("goroutine %d 返回意外错误: %v", i, results[i])
		}
	}
	if succeeded != 1 || stale != 1 {
		t.Fatalf("succeeded=%d stale=%d, want 各 1", succeeded, stale)
	}

	// 无双写：本页 revision 恰好 2 条（base + winner），current 指向胜者
	if n := countRows(t, tdb, "revision", "page_id = $1", p.ID); n != 2 {
		t.Fatalf("revision 数 = %d, want 2", n)
	}
	if n := countRows(t, tdb, "outbox_event", "aggregate_id = $1 AND event_type = 'page.revision_published'", p.ID); n != 2 {
		t.Fatalf("revision_published outbox 数 = %d, want 2（首发布+胜者各一；page.created 不计）", n)
	}
	got, err := svc.GetPage(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *got.CurrentRevisionID != winner.ID {
		t.Fatalf("current = %v, want 胜者 %s", got.CurrentRevisionID, winner.ID)
	}
	if winner.ParentRevisionID == nil || *winner.ParentRevisionID != rev1.ID {
		t.Fatalf("胜者 parent = %v, want %s", winner.ParentRevisionID, rev1.ID)
	}
}

func TestPublish_InvalidASTRejected(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	p := createMainPage(t, svc, "AST Target", actor)
	ctx := context.Background()

	cases := map[string]json.RawMessage{
		"非 JSON":           json.RawMessage(`{oops`),
		"非对象":              json.RawMessage(`[1,2]`),
		"schema_version=2": json.RawMessage(`{"type":"document","schema_version":2,"children":[]}`),
		"错误 type":          json.RawMessage(`{"type":"doc","schema_version":1,"children":[]}`),
		"未知 Block 类型":      json.RawMessage(`{"type":"document","schema_version":1,"children":[{"id":"00000000-0000-7000-8000-000000000901","type":"video"}]}`),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Publish(ctx, page.PublishParams{PageID: p.ID, ActorID: actor, AST: raw})
			if !errors.Is(err, page.ErrInvalidAST) {
				t.Fatalf("err = %v, want ErrInvalidAST", err)
			}
		})
	}

	// 库中无发布残留（page.created 的审计/Outbox 事件属创建动作，不在本断言范围）
	for _, table := range []string{"content_snapshot", "revision"} {
		if n := countRows(t, tdb, table, ""); n != 0 {
			t.Fatalf("%s 残留 %d 行", table, n)
		}
	}
	if n := countRows(t, tdb, "audit_event", "event_type = $1", page.EventTypeRevisionPublished); n != 0 {
		t.Fatalf("audit_event 残留 %d 行 revision.published", n)
	}
	if n := countRows(t, tdb, "outbox_event", "event_type = $1", page.OutboxEventRevisionPublished); n != 0 {
		t.Fatalf("outbox_event 残留 %d 行 page.revision_published", n)
	}
	got, err := svc.GetPage(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentRevisionID != nil {
		t.Fatalf("current 应为 nil, 得到 %v", got.CurrentRevisionID)
	}
}

func TestPublish_ActorRules(t *testing.T) {
	tdb, svc := setup(t)
	human := tdb.MakeActor(t, "human", "alice")
	p := createMainPage(t, svc, "Actor Page", human)
	ctx := context.Background()

	// ai actor 直接发布被拒绝
	ai := tdb.MakeActor(t, "ai", "gpt")
	_, err := svc.Publish(ctx, page.PublishParams{PageID: p.ID, ActorID: ai, AST: validAST(t, "x")})
	if !errors.Is(err, page.ErrActorNotAllowed) {
		t.Fatalf("err = %v, want ErrActorNotAllowed", err)
	}
	// 不存在的 actor
	_, err = svc.Publish(ctx, page.PublishParams{PageID: p.ID, ActorID: uuid.New(), AST: validAST(t, "x")})
	if !errors.Is(err, page.ErrInvalidActor) {
		t.Fatalf("err = %v, want ErrInvalidActor", err)
	}
	// bot / system 允许
	bot := tdb.MakeActor(t, "bot", "importer")
	publishOne(t, svc, p.ID, bot, nil, validAST(t, "bot 发布"))
	if n := countRows(t, tdb, "revision", "page_id = $1", p.ID); n != 1 {
		t.Fatalf("ai/无效 actor 的发布不应落库, revision 数 = %d, want 1", n)
	}
}

func TestPublish_PageNotFound(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	_, err := svc.Publish(ctx, page.PublishParams{PageID: uuid.New(), ActorID: actor, AST: validAST(t, "x")})
	if !errors.Is(err, page.ErrPageNotFound) {
		t.Fatalf("err = %v, want ErrPageNotFound", err)
	}

	p := createMainPage(t, svc, "Doomed Page", actor)
	tdb.SoftDeletePage(t, p.ID)
	_, err = svc.Publish(ctx, page.PublishParams{PageID: p.ID, ActorID: actor, AST: validAST(t, "x")})
	if !errors.Is(err, page.ErrPageNotFound) {
		t.Fatalf("err = %v, want ErrPageNotFound（软删除页）", err)
	}
}
