package knowledge_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

// setup 建连 + Reset，并装配领域服务（注入 evidence 的 citation 存在性只读检查，
// M4-T05：knowledge 定义 CitationChecker 接口，evidence.Repository 实现）。
func setup(t *testing.T) (*testkit.DB, *knowledge.Service) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	svc := knowledge.NewService(
		knowledge.NewRepository(tdb.Pool),
		page.NewRepository(tdb.Pool),
		db.NewTxManager(tdb.Pool),
		id.NewGenerator(),
	).WithCitationChecker(evidence.NewRepository(tdb.Pool))
	return tdb, svc
}

// createEntity 以单个 zh-Hans 主标签创建实体（跳过断言样板）。
func createEntity(t *testing.T, svc *knowledge.Service, typeKey, canonicalKey, label string, actorID uuid.UUID) *knowledge.Entity {
	t.Helper()
	e, err := svc.CreateEntity(context.Background(), knowledge.CreateEntityParams{
		WikiID:       testkit.DefaultWikiID,
		TypeKey:      typeKey,
		CanonicalKey: canonicalKey,
		Labels: []knowledge.LabelInput{
			{Language: "zh-Hans", Label: label, IsPrimary: true},
		},
		ActorID: actorID,
	})
	if err != nil {
		t.Fatalf("CreateEntity(%q) 失败: %v", canonicalKey, err)
	}
	return e
}

func TestCreateEntity_Basic(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")

	e, err := svc.CreateEntity(context.Background(), knowledge.CreateEntityParams{
		WikiID:       testkit.DefaultWikiID,
		TypeKey:      "person",
		CanonicalKey: "  Anby   Demara ",
		Labels: []knowledge.LabelInput{
			{Language: "zh-Hans", Label: " 安比·德玛拉 ", Description: "看板娘", IsPrimary: true},
			{Language: "en", Label: "Anby Demara"},
		},
		ActorID: actor,
	})
	if err != nil {
		t.Fatalf("CreateEntity 失败: %v", err)
	}
	if e.ID == uuid.Nil {
		t.Fatal("Entity ID 不应为 Nil")
	}
	if e.CanonicalKey != "anby demara" {
		t.Fatalf("canonical_key = %q", e.CanonicalKey)
	}
	if e.EntityTypeID != testkit.EntityTypePersonID {
		t.Fatalf("entity_type_id = %s, 期望 person 种子 %s", e.EntityTypeID, testkit.EntityTypePersonID)
	}
	if e.Status != knowledge.StatusActive || e.MergedIntoEntityID != nil {
		t.Fatalf("状态异常: status=%q merged_into=%v", e.Status, e.MergedIntoEntityID)
	}
	if e.CreatedBy != actor {
		t.Fatalf("created_by = %s", e.CreatedBy)
	}

	labels, err := svc.ListLabels(context.Background(), e.ID)
	if err != nil {
		t.Fatalf("ListLabels 失败: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("标签数 = %d, 期望 2", len(labels))
	}
	// 标签落库前已 trim + 折叠空白（保留大小写）。
	if labels[1].Label != "安比·德玛拉" || !labels[1].IsPrimary || labels[1].Description != "看板娘" {
		t.Fatalf("主标签异常: %+v", labels[1])
	}
	if labels[0].Label != "Anby Demara" || labels[0].IsPrimary {
		t.Fatalf("次标签异常: %+v", labels[0])
	}
}

func TestCreateEntity_DeriveCanonicalKeyFromPrimaryLabel(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")

	e, err := svc.CreateEntity(context.Background(), knowledge.CreateEntityParams{
		WikiID:  testkit.DefaultWikiID,
		TypeKey: "character",
		Labels: []knowledge.LabelInput{
			{Language: "zh-Hans", Label: "星见雅", IsPrimary: true},
			{Language: "en", Label: "Hoshimi Miyabi", IsPrimary: true},
		},
		ActorID: actor,
	})
	if err != nil {
		t.Fatalf("CreateEntity 失败: %v", err)
	}
	if e.CanonicalKey != "星见雅" {
		t.Fatalf("canonical_key = %q, 期望从首个主标签推导", e.CanonicalKey)
	}
}

func TestCreateEntity_DuplicateCanonicalKey(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	createEntity(t, svc, "person", "anby demara", "安比", actor)

	// 规范化后相同（大小写/空白书写差异）同样冲突。
	_, err := svc.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID:       testkit.DefaultWikiID,
		TypeKey:      "person",
		CanonicalKey: "ANBY   Demara",
		Labels:       []knowledge.LabelInput{{Language: "zh-Hans", Label: "另一个安比", IsPrimary: true}},
		ActorID:      actor,
	})
	if !errors.Is(err, knowledge.ErrDuplicateEntityKey) {
		t.Fatalf("err = %v, 期望 ErrDuplicateEntityKey", err)
	}
}

func TestCreateEntity_UnknownType(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")

	_, err := svc.CreateEntity(context.Background(), knowledge.CreateEntityParams{
		WikiID:  testkit.DefaultWikiID,
		TypeKey: "not-a-type",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "X", IsPrimary: true}},
		ActorID: actor,
	})
	if !errors.Is(err, knowledge.ErrEntityTypeNotFound) {
		t.Fatalf("err = %v, 期望 ErrEntityTypeNotFound", err)
	}
}

func TestCreateEntity_LabelInvariants(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	// 无标签。
	_, err := svc.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "person", ActorID: actor,
	})
	if !errors.Is(err, knowledge.ErrNoPrimaryLabel) {
		t.Fatalf("无标签 err = %v, 期望 ErrNoPrimaryLabel", err)
	}

	// 有标签但都不是主标签。
	_, err = svc.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID:  testkit.DefaultWikiID,
		TypeKey: "person",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "安比"}},
		ActorID: actor,
	})
	if !errors.Is(err, knowledge.ErrNoPrimaryLabel) {
		t.Fatalf("无主标签 err = %v, 期望 ErrNoPrimaryLabel", err)
	}

	// 同语言两个主标签。
	_, err = svc.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID:  testkit.DefaultWikiID,
		TypeKey: "person",
		Labels: []knowledge.LabelInput{
			{Language: "zh-Hans", Label: "安比", IsPrimary: true},
			{Language: "zh-Hans", Label: "安比酱", IsPrimary: true},
		},
		ActorID: actor,
	})
	if !errors.Is(err, knowledge.ErrDuplicatePrimaryLabel) {
		t.Fatalf("双主标签 err = %v, 期望 ErrDuplicatePrimaryLabel", err)
	}

	// 重复 (language, label)。
	_, err = svc.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID:  testkit.DefaultWikiID,
		TypeKey: "person",
		Labels: []knowledge.LabelInput{
			{Language: "zh-Hans", Label: "安比", IsPrimary: true},
			{Language: "zh-Hans", Label: " 安比 "},
		},
		ActorID: actor,
	})
	if !errors.Is(err, knowledge.ErrLabelExists) {
		t.Fatalf("重复标签 err = %v, 期望 ErrLabelExists", err)
	}
}

func TestCreateEntity_AIActorRejected(t *testing.T) {
	tdb, svc := setup(t)
	ai := tdb.MakeActor(t, "ai", "wiki-bot")
	ctx := context.Background()

	_, err := svc.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID:  testkit.DefaultWikiID,
		TypeKey: "person",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "AI 造物", IsPrimary: true}},
		ActorID: ai,
	})
	if !errors.Is(err, page.ErrActorNotAllowed) {
		t.Fatalf("AI actor err = %v, 期望 page.ErrActorNotAllowed", err)
	}

	// 不存在的 actor。
	_, err = svc.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID:  testkit.DefaultWikiID,
		TypeKey: "person",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "幽灵造物", IsPrimary: true}},
		ActorID: uuid.MustParse("00000000-0000-7000-8000-999999999999"),
	})
	if !errors.Is(err, page.ErrInvalidActor) {
		t.Fatalf("未知 actor err = %v, 期望 page.ErrInvalidActor", err)
	}

	// bot/system 允许（与 page 准入规则一致）。
	for _, actorType := range []string{"bot", "system"} {
		a := tdb.MakeActor(t, actorType, actorType+"-writer")
		if _, err := svc.CreateEntity(ctx, knowledge.CreateEntityParams{
			WikiID:  testkit.DefaultWikiID,
			TypeKey: "concept",
			Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: actorType + " 造物", IsPrimary: true}},
			ActorID: a,
		}); err != nil {
			t.Fatalf("%s actor 应允许创建: %v", actorType, err)
		}
	}
}

func TestAddLabel_PrimaryUniqueness(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	e := createEntity(t, svc, "person", "anby", "安比", actor)

	// 同语言第二个主标签：服务层前置拒绝。
	_, err := svc.AddLabel(ctx, e.ID, knowledge.LabelInput{Language: "zh-Hans", Label: "安比酱", IsPrimary: true})
	if !errors.Is(err, knowledge.ErrDuplicatePrimaryLabel) {
		t.Fatalf("err = %v, 期望 ErrDuplicatePrimaryLabel", err)
	}

	// 非主标签可自由追加；另一语言的主标签互不冲突。
	if _, err := svc.AddLabel(ctx, e.ID, knowledge.LabelInput{Language: "zh-Hans", Label: "安比酱"}); err != nil {
		t.Fatalf("追加非主标签失败: %v", err)
	}
	if _, err := svc.AddLabel(ctx, e.ID, knowledge.LabelInput{Language: "en", Label: "Anby", IsPrimary: true}); err != nil {
		t.Fatalf("追加 en 主标签失败: %v", err)
	}

	// 重复 (language, label) 拒绝（含书写差异）。
	if _, err := svc.AddLabel(ctx, e.ID, knowledge.LabelInput{Language: "zh-Hans", Label: " 安比 "}); err == nil || !errors.Is(err, knowledge.ErrLabelExists) {
		t.Fatalf("err = %v, 期望 ErrLabelExists", err)
	}
}

func TestAddLabel_ConcurrentDoublePrimary(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	// 只有 en 主标签：zh-Hans 主标签虚位以待，两个并发写竞争同一个位置。
	e, err := svc.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID:       testkit.DefaultWikiID,
		TypeKey:      "person",
		CanonicalKey: "anby",
		Labels:       []knowledge.LabelInput{{Language: "en", Label: "Anby", IsPrimary: true}},
		ActorID:      actor,
	})
	if err != nil {
		t.Fatalf("CreateEntity 失败: %v", err)
	}

	// 并发追加同语言主标签：实体行锁 + DB 部分唯一索引保证只有一个成功。
	var wg sync.WaitGroup
	errs := make([]error, 2)
	labels := []string{"安比酱", "小安比"}
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.AddLabel(ctx, e.ID, knowledge.LabelInput{Language: "zh-Hans", Label: labels[i], IsPrimary: true})
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, knowledge.ErrDuplicatePrimaryLabel) {
			t.Fatalf("失败方 err = %v, 期望 ErrDuplicatePrimaryLabel", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("成功数 = %d, 期望恰好 1", succeeded)
	}

	// 最终仍只有一个 zh-Hans 主标签。
	all, err := svc.ListLabels(ctx, e.ID)
	if err != nil {
		t.Fatalf("ListLabels 失败: %v", err)
	}
	primaryCount := 0
	for _, l := range all {
		if l.Language == "zh-Hans" && l.IsPrimary {
			primaryCount++
		}
	}
	if primaryCount != 1 {
		t.Fatalf("zh-Hans 主标签数 = %d, 期望 1", primaryCount)
	}
}

func TestSetPrimaryLabel(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	e := createEntity(t, svc, "person", "anby", "安比", actor)

	if _, err := svc.AddLabel(ctx, e.ID, knowledge.LabelInput{Language: "zh-Hans", Label: "安比酱"}); err != nil {
		t.Fatalf("AddLabel 失败: %v", err)
	}
	if err := svc.SetPrimaryLabel(ctx, e.ID, "zh-Hans", "安比酱"); err != nil {
		t.Fatalf("SetPrimaryLabel 失败: %v", err)
	}

	all, err := svc.ListLabels(ctx, e.ID)
	if err != nil {
		t.Fatalf("ListLabels 失败: %v", err)
	}
	primaryOf := map[string]bool{}
	for _, l := range all {
		primaryOf[l.Label] = l.IsPrimary
	}
	if !primaryOf["安比酱"] || primaryOf["安比"] {
		t.Fatalf("主标签未切换: %+v", all)
	}

	// 已是主标签：no-op。
	if err := svc.SetPrimaryLabel(ctx, e.ID, "zh-Hans", "安比酱"); err != nil {
		t.Fatalf("重复 SetPrimaryLabel 应为 no-op: %v", err)
	}
	// 目标标签不存在。
	if err := svc.SetPrimaryLabel(ctx, e.ID, "zh-Hans", "不存在"); !errors.Is(err, knowledge.ErrLabelNotFound) {
		t.Fatalf("err = %v, 期望 ErrLabelNotFound", err)
	}
}

func TestRemoveLabel(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	e := createEntity(t, svc, "person", "anby", "安比", actor)
	if _, err := svc.AddLabel(ctx, e.ID, knowledge.LabelInput{Language: "en", Label: "Anby", IsPrimary: true}); err != nil {
		t.Fatalf("AddLabel 失败: %v", err)
	}
	if _, err := svc.AddLabel(ctx, e.ID, knowledge.LabelInput{Language: "zh-Hans", Label: "安比酱"}); err != nil {
		t.Fatalf("AddLabel 失败: %v", err)
	}

	// 删除非主标签。
	if err := svc.RemoveLabel(ctx, e.ID, "zh-Hans", "安比酱"); err != nil {
		t.Fatalf("RemoveLabel 失败: %v", err)
	}
	// 删除不存在的标签。
	if err := svc.RemoveLabel(ctx, e.ID, "zh-Hans", "安比酱"); !errors.Is(err, knowledge.ErrLabelNotFound) {
		t.Fatalf("err = %v, 期望 ErrLabelNotFound", err)
	}
	// zh-Hans 主标签可删（en 主标签仍在）。
	if err := svc.RemoveLabel(ctx, e.ID, "zh-Hans", "安比"); err != nil {
		t.Fatalf("RemoveLabel(zh 主标签) 失败: %v", err)
	}
	// en 主标签是仅剩的主标签：拒绝删除。
	if err := svc.RemoveLabel(ctx, e.ID, "en", "Anby"); !errors.Is(err, knowledge.ErrNoPrimaryLabel) {
		t.Fatalf("err = %v, 期望 ErrNoPrimaryLabel", err)
	}
}

func TestAlias_AddNormalizeRemove(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	e := createEntity(t, svc, "character", "hoshimi-miyabi", "星见雅", actor)

	a, err := svc.AddAlias(ctx, e.ID, knowledge.AliasInput{Language: "en", Alias: "  Hoshimi   Miyabi "})
	if err != nil {
		t.Fatalf("AddAlias 失败: %v", err)
	}
	if a.Alias != "Hoshimi Miyabi" {
		t.Fatalf("alias 展示形态 = %q", a.Alias)
	}
	if a.NormalizedAlias != "hoshimi miyabi" {
		t.Fatalf("normalized_alias = %q", a.NormalizedAlias)
	}
	if a.AliasType != knowledge.AliasTypeCommon {
		t.Fatalf("alias_type = %q, 期望默认 common", a.AliasType)
	}

	// 规范化后重复（大小写/空白差异）拒绝。
	if _, err := svc.AddAlias(ctx, e.ID, knowledge.AliasInput{Language: "en", Alias: "HOSHIMI miyabi"}); !errors.Is(err, knowledge.ErrDuplicateAlias) {
		t.Fatalf("err = %v, 期望 ErrDuplicateAlias", err)
	}
	// 不同别名可追加。
	second, err := svc.AddAlias(ctx, e.ID, knowledge.AliasInput{Language: "zh-Hans", Alias: "雅小姐", AliasType: knowledge.AliasTypeHistorical})
	if err != nil {
		t.Fatalf("AddAlias(雅小姐) 失败: %v", err)
	}

	aliases, err := svc.ListAliases(ctx, e.ID)
	if err != nil {
		t.Fatalf("ListAliases 失败: %v", err)
	}
	if len(aliases) != 2 {
		t.Fatalf("别名数 = %d, 期望 2", len(aliases))
	}

	// 删除与重复删除。
	if err := svc.RemoveAlias(ctx, e.ID, second.ID); err != nil {
		t.Fatalf("RemoveAlias 失败: %v", err)
	}
	if err := svc.RemoveAlias(ctx, e.ID, second.ID); !errors.Is(err, knowledge.ErrAliasNotFound) {
		t.Fatalf("err = %v, 期望 ErrAliasNotFound", err)
	}
}

// 搜索fixture：canonical/label/alias 三种命中 + 一个 fuzzy 命中。
func seedSearchEntities(t *testing.T, svc *knowledge.Service, actor uuid.UUID) (canonical, label, alias, fuzzy *knowledge.Entity) {
	t.Helper()
	ctx := context.Background()
	canonical = createEntity(t, svc, "person", "anby demara", "安比", actor)
	label = createEntity(t, svc, "person", "anby-clone", "安比二号", actor)
	if _, err := svc.AddLabel(ctx, label.ID, knowledge.LabelInput{Language: "en", Label: "Anby Demara"}); err != nil {
		t.Fatalf("AddLabel 失败: %v", err)
	}
	alias = createEntity(t, svc, "person", "anby-iii", "安比三号", actor)
	if _, err := svc.AddAlias(ctx, alias.ID, knowledge.AliasInput{Language: "en", Alias: "Anby Demara"}); err != nil {
		t.Fatalf("AddAlias 失败: %v", err)
	}
	fuzzy = createEntity(t, svc, "person", "anby-figure", "Anby Demara 手办", actor)
	return canonical, label, alias, fuzzy
}

func TestSearchEntities_MatchedOnAndOrdering(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	canonical, label, alias, fuzzy := seedSearchEntities(t, svc, actor)

	// 查询串带书写差异，规范化后匹配。
	results, err := svc.SearchEntities(context.Background(), knowledge.SearchParams{
		WikiID: testkit.DefaultWikiID,
		Query:  "  ANBY   Demara ",
	})
	if err != nil {
		t.Fatalf("SearchEntities 失败: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("结果数 = %d, 期望 4: %+v", len(results), results)
	}

	want := []struct {
		id        uuid.UUID
		matchedOn string
		exact     bool
	}{
		{canonical.ID, knowledge.MatchedOnCanonical, true},
		{label.ID, knowledge.MatchedOnLabel, true},
		{alias.ID, knowledge.MatchedOnAlias, true},
		{fuzzy.ID, knowledge.MatchedOnLabel, false},
	}
	for i, w := range want {
		got := results[i]
		if got.Entity.ID != w.id || got.MatchedOn != w.matchedOn || got.Exact != w.exact {
			t.Fatalf("results[%d] = {id=%s matched_on=%q exact=%v}, 期望 {id=%s matched_on=%q exact=%v}",
				i, got.Entity.ID, got.MatchedOn, got.Exact, w.id, w.matchedOn, w.exact)
		}
	}
}

// MT-M4-NO-AUTOMERGE：同名（标签相同）实体不自动合并，创建放行，搜索返回全部候选。
func TestSearchEntities_NoAutoMerge(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")

	e1 := createEntity(t, svc, "person", "li-si-1", "李四", actor)
	e2 := createEntity(t, svc, "person", "li-si-2", "李四", actor)
	if e1.ID == e2.ID {
		t.Fatal("同名实体被错误合并")
	}

	results, err := svc.SearchEntities(context.Background(), knowledge.SearchParams{
		WikiID: testkit.DefaultWikiID,
		Query:  "李四",
	})
	if err != nil {
		t.Fatalf("SearchEntities 失败: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("同名候选数 = %d, 期望 2（不自动合并）", len(results))
	}
	got := map[uuid.UUID]bool{}
	for _, r := range results {
		got[r.Entity.ID] = true
		if !r.Exact || r.MatchedOn != knowledge.MatchedOnLabel {
			t.Fatalf("命中标记异常: matched_on=%q exact=%v", r.MatchedOn, r.Exact)
		}
	}
	if !got[e1.ID] || !got[e2.ID] {
		t.Fatalf("搜索结果未包含全部同名候选: %v", got)
	}
}

func TestSearchEntities_TypeFilterAndLimit(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	createEntity(t, svc, "person", "anby", "安比", actor)
	createEntity(t, svc, "place", "anby-street", "安比街", actor)
	createEntity(t, svc, "place", "anby-city", "安比城", actor)

	// 类型过滤。
	results, err := svc.SearchEntities(ctx, knowledge.SearchParams{
		WikiID: testkit.DefaultWikiID, Query: "安比", TypeKey: "place",
	})
	if err != nil {
		t.Fatalf("SearchEntities 失败: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("place 过滤结果数 = %d, 期望 2", len(results))
	}
	for _, r := range results {
		if r.Entity.EntityTypeID != testkit.EntityTypePlaceID {
			t.Fatalf("类型过滤失效: %+v", r.Entity)
		}
	}

	// Limit 截断。
	results, err = svc.SearchEntities(ctx, knowledge.SearchParams{
		WikiID: testkit.DefaultWikiID, Query: "安比", Limit: 2,
	})
	if err != nil {
		t.Fatalf("SearchEntities 失败: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Limit=2 结果数 = %d", len(results))
	}

	// 空查询拒绝。
	if _, err := svc.SearchEntities(ctx, knowledge.SearchParams{WikiID: testkit.DefaultWikiID, Query: "  "}); !errors.Is(err, knowledge.ErrInvalidLabel) {
		t.Fatalf("空查询 err = %v, 期望 ErrInvalidLabel", err)
	}
}

func TestSearchEntities_MergedVisibility(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	merged := createEntity(t, svc, "person", "anby-old", "安比旧", actor)
	target := createEntity(t, svc, "person", "anby-new", "安比新", actor)
	tdb.SetEntityMerged(t, merged.ID, target.ID)

	// 默认排除 merged。
	results, err := svc.SearchEntities(ctx, knowledge.SearchParams{WikiID: testkit.DefaultWikiID, Query: "安比旧"})
	if err != nil {
		t.Fatalf("SearchEntities 失败: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("默认搜索应排除 merged 实体，得到 %d 条", len(results))
	}

	// IncludeMerged 可查到。
	results, err = svc.SearchEntities(ctx, knowledge.SearchParams{
		WikiID: testkit.DefaultWikiID, Query: "安比旧", IncludeMerged: true,
	})
	if err != nil {
		t.Fatalf("SearchEntities 失败: %v", err)
	}
	if len(results) != 1 || results[0].Entity.ID != merged.ID {
		t.Fatalf("IncludeMerged 应返回 merged 实体: %+v", results)
	}
}

func TestBindPage_Lifecycle(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	e := createEntity(t, svc, "person", "anby", "安比", actor)
	pageID := tdb.MakePage(t, testkit.MainNamespaceID, "anby demara", "Anby Demara", actor)

	// primary + mentioned 绑定。
	if _, err := svc.BindPage(ctx, knowledge.BindPageParams{
		PageID: pageID, EntityID: e.ID, Role: knowledge.BindingRolePrimary, Language: "zh-Hans",
	}); err != nil {
		t.Fatalf("BindPage(primary) 失败: %v", err)
	}
	if _, err := svc.BindPage(ctx, knowledge.BindPageParams{
		PageID: pageID, EntityID: e.ID, Role: knowledge.BindingRoleMentioned, Language: "zh-Hans",
	}); err != nil {
		t.Fatalf("BindPage(mentioned) 失败: %v", err)
	}

	// 重复绑定拒绝。
	if _, err := svc.BindPage(ctx, knowledge.BindPageParams{
		PageID: pageID, EntityID: e.ID, Role: knowledge.BindingRolePrimary, Language: "zh-Hans",
	}); !errors.Is(err, knowledge.ErrBindingExists) {
		t.Fatalf("err = %v, 期望 ErrBindingExists", err)
	}

	// 非法角色。
	if _, err := svc.BindPage(ctx, knowledge.BindPageParams{
		PageID: pageID, EntityID: e.ID, Role: "owner", Language: "zh-Hans",
	}); !errors.Is(err, knowledge.ErrInvalidBindingRole) {
		t.Fatalf("err = %v, 期望 ErrInvalidBindingRole", err)
	}

	// 解绑与重复解绑。
	if err := svc.UnbindPage(ctx, pageID, e.ID, knowledge.BindingRoleMentioned); err != nil {
		t.Fatalf("UnbindPage 失败: %v", err)
	}
	if err := svc.UnbindPage(ctx, pageID, e.ID, knowledge.BindingRoleMentioned); !errors.Is(err, knowledge.ErrBindingNotFound) {
		t.Fatalf("err = %v, 期望 ErrBindingNotFound", err)
	}
}

func TestBindPage_ExistenceChecks(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	e := createEntity(t, svc, "person", "anby", "安比", actor)
	pageID := tdb.MakePage(t, testkit.MainNamespaceID, "anby demara", "Anby Demara", actor)

	// 页面不存在。
	if _, err := svc.BindPage(ctx, knowledge.BindPageParams{
		PageID: tdb.NewID(t), EntityID: e.ID, Role: knowledge.BindingRolePrimary, Language: "zh-Hans",
	}); !errors.Is(err, page.ErrPageNotFound) {
		t.Fatalf("err = %v, 期望 page.ErrPageNotFound", err)
	}

	// 页面已软删除。
	tdb.SoftDeletePage(t, pageID)
	if _, err := svc.BindPage(ctx, knowledge.BindPageParams{
		PageID: pageID, EntityID: e.ID, Role: knowledge.BindingRolePrimary, Language: "zh-Hans",
	}); !errors.Is(err, page.ErrPageNotFound) {
		t.Fatalf("err = %v, 期望 page.ErrPageNotFound（软删除页面）", err)
	}

	// 实体不存在。
	livePageID := tdb.MakePage(t, testkit.MainNamespaceID, "anby 2", "Anby 2", actor)
	if _, err := svc.BindPage(ctx, knowledge.BindPageParams{
		PageID: livePageID, EntityID: tdb.NewID(t), Role: knowledge.BindingRolePrimary, Language: "zh-Hans",
	}); !errors.Is(err, knowledge.ErrEntityNotFound) {
		t.Fatalf("err = %v, 期望 ErrEntityNotFound", err)
	}
}

func TestMergedEntity_WriteRejected(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()

	merged := createEntity(t, svc, "person", "anby-old", "安比旧", actor)
	target := createEntity(t, svc, "person", "anby-new", "安比新", actor)
	alias, err := svc.AddAlias(ctx, merged.ID, knowledge.AliasInput{Language: "zh-Hans", Alias: "旧安比"})
	if err != nil {
		t.Fatalf("AddAlias 失败: %v", err)
	}
	if _, err := svc.AddLabel(ctx, merged.ID, knowledge.LabelInput{Language: "en", Label: "Anby Old", IsPrimary: true}); err != nil {
		t.Fatalf("AddLabel 失败: %v", err)
	}
	pageID := tdb.MakePage(t, testkit.MainNamespaceID, "anby", "Anby", actor)

	tdb.SetEntityMerged(t, merged.ID, target.ID)

	// 全部写操作拒绝。
	if _, err := svc.AddLabel(ctx, merged.ID, knowledge.LabelInput{Language: "zh-Hans", Label: "新标签"}); !errors.Is(err, knowledge.ErrEntityMerged) {
		t.Fatalf("AddLabel err = %v, 期望 ErrEntityMerged", err)
	}
	if err := svc.SetPrimaryLabel(ctx, merged.ID, "en", "Anby Old"); !errors.Is(err, knowledge.ErrEntityMerged) {
		t.Fatalf("SetPrimaryLabel err = %v, 期望 ErrEntityMerged", err)
	}
	if err := svc.RemoveLabel(ctx, merged.ID, "en", "Anby Old"); !errors.Is(err, knowledge.ErrEntityMerged) {
		t.Fatalf("RemoveLabel err = %v, 期望 ErrEntityMerged", err)
	}
	if _, err := svc.AddAlias(ctx, merged.ID, knowledge.AliasInput{Language: "zh-Hans", Alias: "另一个别名"}); !errors.Is(err, knowledge.ErrEntityMerged) {
		t.Fatalf("AddAlias err = %v, 期望 ErrEntityMerged", err)
	}
	if err := svc.RemoveAlias(ctx, merged.ID, alias.ID); !errors.Is(err, knowledge.ErrEntityMerged) {
		t.Fatalf("RemoveAlias err = %v, 期望 ErrEntityMerged", err)
	}
	if _, err := svc.BindPage(ctx, knowledge.BindPageParams{
		PageID: pageID, EntityID: merged.ID, Role: knowledge.BindingRolePrimary, Language: "zh-Hans",
	}); !errors.Is(err, knowledge.ErrEntityMerged) {
		t.Fatalf("BindPage err = %v, 期望 ErrEntityMerged", err)
	}

	// 目标实体不受影响。
	if _, err := svc.AddLabel(ctx, target.ID, knowledge.LabelInput{Language: "en", Label: "Anby New", IsPrimary: true}); err != nil {
		t.Fatalf("目标实体写入不应受影响: %v", err)
	}
}
