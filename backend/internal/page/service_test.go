package page_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

// setup 建连 + Reset，并装配领域服务。
func setup(t *testing.T) (*testkit.DB, *page.Service) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	svc := page.NewService(
		page.NewRepository(tdb.Pool),
		db.NewTxManager(tdb.Pool),
		id.NewGenerator(),
	)
	return tdb, svc
}

func createMainPage(t *testing.T, svc *page.Service, title string, actorID uuid.UUID) *page.Page {
	t.Helper()
	p, err := svc.CreatePage(context.Background(), page.CreatePageParams{
		WikiID:      testkit.DefaultWikiID,
		NamespaceID: testkit.MainNamespaceID,
		Title:       title,
		ActorID:     actorID,
	})
	if err != nil {
		t.Fatalf("CreatePage(%q) 失败: %v", title, err)
	}
	return p
}

func TestCreatePage_Basic(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")

	p := createMainPage(t, svc, "  Anby   Demara ", actor)

	if p.ID == uuid.Nil {
		t.Fatal("Page ID 不应为 Nil")
	}
	if p.NormalizedTitle != "anby demara" {
		t.Fatalf("normalized_title = %q", p.NormalizedTitle)
	}
	if p.DisplayTitle != "Anby Demara" {
		t.Fatalf("display_title = %q", p.DisplayTitle)
	}
	if p.Status != page.StatusActive || p.DeletedAt != nil {
		t.Fatalf("状态异常: status=%q deleted_at=%v", p.Status, p.DeletedAt)
	}
	if p.CreatedBy != actor {
		t.Fatalf("created_by = %s", p.CreatedBy)
	}
}

func TestCreatePage_NormalizationBoundaries(t *testing.T) {
	tdb, svc := setup(t)
	ctx := context.Background()

	tests := []struct {
		name  string
		raw   string
		want  string
		wantE error
	}{
		{"全角空格折叠", "Foo　　Bar", "foo bar", nil},
		{"NFC 分解形式", "Café", "café", nil},
		{"大小写折叠", "HELLO World", "hello world", nil},
		{"超过 255 字符", strings.Repeat("a", 256), "", page.ErrInvalidTitle},
		{"空白标题", "　　 ", "", page.ErrInvalidTitle},
		{"控制字符", "Foo\x00Bar", "", page.ErrInvalidTitle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tdb.Reset(t) // 用例间隔离，避免标题互相占用
			actor := tdb.MakeActor(t, "human", "alice")
			p, err := svc.CreatePage(ctx, page.CreatePageParams{
				WikiID:      testkit.DefaultWikiID,
				NamespaceID: testkit.MainNamespaceID,
				Title:       tt.raw,
				ActorID:     actor,
			})
			if tt.wantE != nil {
				if !errors.Is(err, tt.wantE) {
					t.Fatalf("err = %v, want %v", err, tt.wantE)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreatePage 失败: %v", err)
			}
			if p.NormalizedTitle != tt.want {
				t.Fatalf("normalized_title = %q, want %q", p.NormalizedTitle, tt.want)
			}
		})
	}
}

func TestCreatePage_TitleConflict(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	createMainPage(t, svc, "Foo Bar", actor)

	// 大小写/空白差异规范化后仍冲突
	_, err := svc.CreatePage(ctx, page.CreatePageParams{
		WikiID:      testkit.DefaultWikiID,
		NamespaceID: testkit.MainNamespaceID,
		Title:       "  foo   BAR ",
		ActorID:     actor,
	})
	if !errors.Is(err, page.ErrTitleConflict) {
		t.Fatalf("err = %v, want ErrTitleConflict", err)
	}
}

func TestRenamePage_OldTitleResolvesViaAlias(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	p := createMainPage(t, svc, "Old Name", actor)
	oldID := p.ID

	renamed, err := svc.RenamePage(ctx, p.ID, "New Name", actor)
	if err != nil {
		t.Fatalf("RenamePage 失败: %v", err)
	}
	if renamed.ID != oldID {
		t.Fatalf("改名后 Page ID 变化: %s -> %s", oldID, renamed.ID)
	}
	if renamed.NormalizedTitle != "new name" || renamed.DisplayTitle != "New Name" {
		t.Fatalf("改名后标题 = %q/%q", renamed.NormalizedTitle, renamed.DisplayTitle)
	}

	// 旧标题经别名解析到同一页面
	res, err := svc.ResolveTitle(ctx, testkit.DefaultWikiID, "main", "OLD  name")
	if err != nil {
		t.Fatalf("ResolveTitle 旧标题失败: %v", err)
	}
	if !res.ViaAlias {
		t.Fatal("旧标题应经由别名解析（ViaAlias=true）")
	}
	if res.Page.ID != oldID {
		t.Fatalf("别名解析到 %s, want %s", res.Page.ID, oldID)
	}

	// 新标题直接命中活页面
	res, err = svc.ResolveTitle(ctx, testkit.DefaultWikiID, "main", "New Name")
	if err != nil {
		t.Fatalf("ResolveTitle 新标题失败: %v", err)
	}
	if res.ViaAlias || res.Page.ID != oldID {
		t.Fatalf("新标题解析异常: ViaAlias=%v ID=%s", res.ViaAlias, res.Page.ID)
	}
}

func TestRenamePage_ConflictWithLivePage(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	createMainPage(t, svc, "Taken", actor)
	p := createMainPage(t, svc, "Other", actor)

	_, err := svc.RenamePage(ctx, p.ID, "TAKEN", actor)
	if !errors.Is(err, page.ErrTitleConflict) {
		t.Fatalf("err = %v, want ErrTitleConflict", err)
	}
}

func TestRenamePage_ConflictWithOtherPageAlias(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	// A 改名后留下别名 "first"
	a := createMainPage(t, svc, "First", actor)
	if _, err := svc.RenamePage(ctx, a.ID, "Second", actor); err != nil {
		t.Fatalf("RenamePage 失败: %v", err)
	}
	b := createMainPage(t, svc, "Third", actor)

	// B 改名为 A 的旧标题 → 别名冲突
	_, err := svc.RenamePage(ctx, b.ID, "first", actor)
	if !errors.Is(err, page.ErrTitleConflict) {
		t.Fatalf("err = %v, want ErrTitleConflict", err)
	}
}

func TestCreatePage_ConflictWithAlias(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	a := createMainPage(t, svc, "Gone Soon", actor)
	if _, err := svc.RenamePage(ctx, a.ID, "Renamed", actor); err != nil {
		t.Fatalf("RenamePage 失败: %v", err)
	}

	// 新建页面占用别名的标题 → 冲突
	_, err := svc.CreatePage(ctx, page.CreatePageParams{
		WikiID:      testkit.DefaultWikiID,
		NamespaceID: testkit.MainNamespaceID,
		Title:       "Gone Soon",
		ActorID:     actor,
	})
	if !errors.Is(err, page.ErrTitleConflict) {
		t.Fatalf("err = %v, want ErrTitleConflict", err)
	}
}

func TestRenamePage_BackToFormerTitleReclaimsAlias(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	p := createMainPage(t, svc, "Alpha", actor)
	if _, err := svc.RenamePage(ctx, p.ID, "Beta", actor); err != nil {
		t.Fatalf("RenamePage 失败: %v", err)
	}
	// 改回曾用名：回收别名，不判冲突，Page ID 不变
	back, err := svc.RenamePage(ctx, p.ID, "ALPHA", actor)
	if err != nil {
		t.Fatalf("改回曾用名失败: %v", err)
	}
	if back.ID != p.ID || back.NormalizedTitle != "alpha" {
		t.Fatalf("改回后 ID=%s title=%q", back.ID, back.NormalizedTitle)
	}
	res, err := svc.ResolveTitle(ctx, testkit.DefaultWikiID, "main", "alpha")
	if err != nil || res.ViaAlias {
		t.Fatalf("改回后应直接命中活页面: res=%+v err=%v", res, err)
	}
	// 旧名 "beta" 现在是别名
	res, err = svc.ResolveTitle(ctx, testkit.DefaultWikiID, "main", "beta")
	if err != nil || !res.ViaAlias || res.Page.ID != p.ID {
		t.Fatalf("beta 应经别名解析: res=%+v err=%v", res, err)
	}
}

func TestResolveTitle_NotFound(t *testing.T) {
	_, svc := setup(t)
	_, err := svc.ResolveTitle(context.Background(), testkit.DefaultWikiID, "main", "no such page")
	if !errors.Is(err, page.ErrPageNotFound) {
		t.Fatalf("err = %v, want ErrPageNotFound", err)
	}
	// 命名空间不存在
	_, err = svc.ResolveTitle(context.Background(), testkit.DefaultWikiID, "no-such-ns", "x")
	if !errors.Is(err, page.ErrNamespaceNotFound) {
		t.Fatalf("err = %v, want ErrNamespaceNotFound", err)
	}
}

func TestRedirect_ChainAndNoRedirect(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	a := createMainPage(t, svc, "RA", actor)
	b := createMainPage(t, svc, "RB", actor)
	c := createMainPage(t, svc, "RC", actor)

	if err := svc.CreateRedirect(ctx, a.ID, b.ID); err != nil {
		t.Fatalf("CreateRedirect A->B 失败: %v", err)
	}
	if err := svc.CreateRedirect(ctx, b.ID, c.ID); err != nil {
		t.Fatalf("CreateRedirect B->C 失败: %v", err)
	}

	final, err := svc.ResolveRedirect(ctx, a.ID, 5)
	if err != nil {
		t.Fatalf("ResolveRedirect 失败: %v", err)
	}
	if final.ID != c.ID {
		t.Fatalf("链式解析到 %s, want %s", final.ID, c.ID)
	}

	// 无重定向页面返回自身
	final, err = svc.ResolveRedirect(ctx, c.ID, 5)
	if err != nil || final.ID != c.ID {
		t.Fatalf("无重定向应返回自身: final=%v err=%v", final, err)
	}
}

func TestRedirect_Loop(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	a := createMainPage(t, svc, "LA", actor)
	b := createMainPage(t, svc, "LB", actor)
	if err := svc.CreateRedirect(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateRedirect(ctx, b.ID, a.ID); err != nil {
		t.Fatal(err)
	}

	_, err := svc.ResolveRedirect(ctx, a.ID, 5)
	if !errors.Is(err, page.ErrRedirectLoop) {
		t.Fatalf("err = %v, want ErrRedirectLoop", err)
	}

	// 自重定向在创建时即拒绝
	err = svc.CreateRedirect(ctx, a.ID, a.ID)
	if !errors.Is(err, page.ErrRedirectLoop) {
		t.Fatalf("err = %v, want ErrRedirectLoop", err)
	}
}

func TestRedirect_TooDeep(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	// 构造 6 页链 p0 -> p1 -> ... -> p5（5 跳可解，maxHops=2 必超限）
	pages := make([]*page.Page, 6)
	for i := range pages {
		pages[i] = createMainPage(t, svc, "Deep "+string(rune('0'+i)), actor)
	}
	for i := 0; i+1 < len(pages); i++ {
		if err := svc.CreateRedirect(ctx, pages[i].ID, pages[i+1].ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ResolveRedirect(ctx, pages[0].ID, 2); !errors.Is(err, page.ErrRedirectTooDeep) {
		t.Fatalf("err = %v, want ErrRedirectTooDeep", err)
	}
	// 默认 5 跳恰好可解 5 边链
	final, err := svc.ResolveRedirect(ctx, pages[0].ID, 0)
	if err != nil || final.ID != pages[5].ID {
		t.Fatalf("默认跳数解析失败: final=%v err=%v", final, err)
	}
}

func TestRedirect_TargetDeleted(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	a := createMainPage(t, svc, "SA", actor)
	b := createMainPage(t, svc, "SB", actor)
	if err := svc.CreateRedirect(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	tdb.SoftDeletePage(t, b.ID)

	_, err := svc.ResolveRedirect(ctx, a.ID, 5)
	if !errors.Is(err, page.ErrRedirectTargetDeleted) {
		t.Fatalf("err = %v, want ErrRedirectTargetDeleted", err)
	}
	// 对已软删除目标再建重定向同样拒绝
	err = svc.CreateRedirect(ctx, a.ID, b.ID)
	if !errors.Is(err, page.ErrRedirectTargetDeleted) {
		t.Fatalf("err = %v, want ErrRedirectTargetDeleted", err)
	}
}

func TestActorValidation(t *testing.T) {
	tdb, svc := setup(t)
	ctx := context.Background()
	params := page.CreatePageParams{
		WikiID:      testkit.DefaultWikiID,
		NamespaceID: testkit.MainNamespaceID,
		Title:       "Actor Check",
	}

	// 不存在的 actor
	params.ActorID = uuid.MustParse("00000000-0000-7000-8000-999999999999")
	if _, err := svc.CreatePage(ctx, params); !errors.Is(err, page.ErrInvalidActor) {
		t.Fatalf("err = %v, want ErrInvalidActor", err)
	}

	// ai actor 直接创建页面被拒绝（M5 治理前）
	params.ActorID = tdb.MakeActor(t, "ai", "gpt")
	if _, err := svc.CreatePage(ctx, params); !errors.Is(err, page.ErrActorNotAllowed) {
		t.Fatalf("err = %v, want ErrActorNotAllowed", err)
	}

	// ai actor 改名同样被拒绝
	human := tdb.MakeActor(t, "human", "alice")
	params.ActorID = human
	p := createMainPage(t, svc, "Actor Target", human)
	ai := tdb.MakeActor(t, "ai", "gpt2")
	if _, err := svc.RenamePage(ctx, p.ID, "Actor Target 2", ai); !errors.Is(err, page.ErrActorNotAllowed) {
		t.Fatalf("err = %v, want ErrActorNotAllowed", err)
	}
	// 无效 actor 改名
	if _, err := svc.RenamePage(ctx, p.ID, "Actor Target 2", uuid.New()); !errors.Is(err, page.ErrInvalidActor) {
		t.Fatalf("err = %v, want ErrInvalidActor", err)
	}

	// bot / system 允许
	bot := tdb.MakeActor(t, "bot", "importer")
	createMainPage(t, svc, "Bot Page", bot)
	createMainPage(t, svc, "System Page", testkit.SystemActorID)
}

// TestPageLifecycleEvents 创建/改名的审计与 Outbox 事件（M3-T04）：
// 与页面写入同事务提交，payload 含 page_id/wiki_id/namespace_id/
// normalized_title/display_title（改名附加 old_normalized_title），
// 驱动 projection 的未解析链接 Resolver。
func TestPageLifecycleEvents(t *testing.T) {
	tdb, svc := setup(t)
	ctx := context.Background()
	actor := tdb.MakeActor(t, "human", "alice")

	p := createMainPage(t, svc, "Venus", actor)

	readEventPayload := func(table, eventType string) map[string]string {
		t.Helper()
		var raw []byte
		q := fmt.Sprintf(`SELECT payload_json::text FROM %s
			WHERE aggregate_id = $1 AND event_type = $2`, table)
		if err := tdb.Pool.QueryRow(ctx, q, p.ID, eventType).Scan(&raw); err != nil {
			t.Fatalf("读取 %s 的 %s 事件失败: %v", table, eventType, err)
		}
		var payload map[string]string
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("解析 %s 的 %s payload 失败: %v", table, eventType, err)
		}
		return payload
	}
	assertPayload := func(table, eventType string, want map[string]string) {
		t.Helper()
		got := readEventPayload(table, eventType)
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("%s %s payload[%q] = %q, want %q（整体: %v）", table, eventType, k, got[k], v, got)
			}
		}
	}

	// 创建：audit_event 与 outbox_event 各一条 page.created，payload 字段完整。
	for _, table := range []string{"audit_event", "outbox_event"} {
		assertPayload(table, page.EventTypePageCreated, map[string]string{
			"page_id":          p.ID.String(),
			"wiki_id":          testkit.DefaultWikiID.String(),
			"namespace_id":     testkit.MainNamespaceID.String(),
			"normalized_title": "venus",
			"display_title":    "Venus",
		})
	}

	// 改名：audit/outbox 各一条 page.renamed，含 old_normalized_title。
	if _, err := svc.RenamePage(ctx, p.ID, "Morning Star", actor); err != nil {
		t.Fatalf("RenamePage 失败: %v", err)
	}
	for _, table := range []string{"audit_event", "outbox_event"} {
		assertPayload(table, page.EventTypePageRenamed, map[string]string{
			"page_id":              p.ID.String(),
			"normalized_title":     "morning star",
			"display_title":        "Morning Star",
			"old_normalized_title": "venus",
		})
	}

	// 仅显示名变化（规范化标题不变）：不发新事件（标题占用无变化）。
	if _, err := svc.RenamePage(ctx, p.ID, "morning  star", actor); err != nil {
		t.Fatalf("仅显示名变化的 RenamePage 失败: %v", err)
	}
	var n int
	if err := tdb.Pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox_event WHERE aggregate_id = $1 AND event_type = $2`,
		p.ID, page.EventTypePageRenamed).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("page.renamed outbox 数 = %d, want 1（仅显示名变化不发事件）", n)
	}
}
