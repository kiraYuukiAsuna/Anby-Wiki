package knowledge_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/testkit"
)

// createClaim 以 human 来源创建 Claim（跳过断言样板）。
func createClaim(t *testing.T, svc *knowledge.Service, subjectID uuid.UUID, propertyKey string, value knowledge.Value, actorID uuid.UUID) *knowledge.Claim {
	t.Helper()
	c, err := svc.CreateClaim(context.Background(), knowledge.CreateClaimParams{
		SubjectEntityID: subjectID,
		PropertyKey:     propertyKey,
		Value:           value,
		OriginType:      knowledge.OriginHuman,
		ActorID:         actorID,
	})
	if err != nil {
		t.Fatalf("CreateClaim(%q) 失败: %v", propertyKey, err)
	}
	return c
}

// TestCreateClaim_ValueShapesAndInitialStatus 全值类型经服务落库的形态 + 初始状态映射。
func TestCreateClaim_ValueShapesAndInitialStatus(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	subject := createEntity(t, svc, "software", "zzz", "绝区零", actor)
	target := createEntity(t, svc, "organization", "miHoYo", "米哈游", actor)

	// 种子之外值类型用自定义 property 覆盖（种子只有 entity/date）。
	tdb.MakeProperty(t, "tagline", "string", false, nil, nil, "")
	tdb.MakeProperty(t, "score", "number", false, nil, nil, "")
	tdb.MakeProperty(t, "hq_location", "coordinate", false, nil, nil, "")
	tdb.MakeProperty(t, "crew_size", "composite", false, nil, nil,
		`{"value":{"required":["count"],"properties":{"count":{"type":"number"}}}}`)

	tests := []struct {
		name       string
		property   string
		value      knowledge.Value
		wantJSON   string
		wantTarget *uuid.UUID
	}{
		{"string", "tagline", knowledge.StringValue("动作游戏"), `"动作游戏"`, nil},
		{"number", "score", knowledge.NumberValue(9.5), `9.5`, nil},
		{"date", "release_date", knowledge.DateValue("2024-07-04"), `"2024-07-04"`, nil},
		{"entity", "developer", knowledge.EntityValue(target.ID),
			`{"entity_id":"` + target.ID.String() + `"}`, &target.ID},
		{"coordinate", "hq_location", knowledge.CoordinateValue(31.23, 121.47), `{"lat":31.23,"lon":121.47}`, nil},
		{"composite", "crew_size", knowledge.CompositeValue(json.RawMessage(`{"count":300}`)), `{"count":300}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := createClaim(t, svc, subject.ID, tt.property, tt.value, actor)
			if string(c.ValueJSON) != tt.wantJSON {
				t.Fatalf("value_json = %s, 期望 %s", c.ValueJSON, tt.wantJSON)
			}
			if (c.TargetEntityID == nil) != (tt.wantTarget == nil) {
				t.Fatalf("target_entity_id = %v, 期望 %v", c.TargetEntityID, tt.wantTarget)
			}
			if c.TargetEntityID != nil && *c.TargetEntityID != *tt.wantTarget {
				t.Fatalf("target_entity_id = %s, 期望 %s", c.TargetEntityID, tt.wantTarget)
			}
			if c.Status != knowledge.ClaimStatusPublished {
				t.Fatalf("human 来源初始状态 = %q, 期望 published", c.Status)
			}
			if c.VerificationStatus != knowledge.VerificationUnverified {
				t.Fatalf("初始验证状态 = %q, 期望 unverified", c.VerificationStatus)
			}
			if c.Rank != knowledge.RankNormal || string(c.QualifiersJSON) != `{}` {
				t.Fatalf("缺省 rank/qualifiers 异常: rank=%q qualifiers=%s", c.Rank, c.QualifiersJSON)
			}
		})
	}

	// 初始状态映射：ai/import → proposed。
	for _, origin := range []string{knowledge.OriginAI, knowledge.OriginImport} {
		c, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
			SubjectEntityID: subject.ID, PropertyKey: "score",
			Value: knowledge.NumberValue(1), OriginType: origin, ActorID: actor,
		})
		if err == nil || !errors.Is(err, knowledge.ErrClaimNotMultivalued) {
			// score 单值已有 published：先验证错误路径有效（防止误配置成多值）。
			t.Fatalf("score 二次创建 err = %v, 期望 ErrClaimNotMultivalued", err)
		}
		tdb.MakeProperty(t, "ai_prop_"+origin, "string", true, nil, nil, "")
		c, err = svc.CreateClaim(ctx, knowledge.CreateClaimParams{
			SubjectEntityID: subject.ID, PropertyKey: "ai_prop_" + origin,
			Value: knowledge.StringValue("x"), OriginType: origin, ActorID: actor,
		})
		if err != nil {
			t.Fatalf("%s 来源创建失败: %v", origin, err)
		}
		if c.Status != knowledge.ClaimStatusProposed {
			t.Fatalf("%s 来源初始状态 = %q, 期望 proposed", origin, c.Status)
		}
	}

	// 有效时间落库（INV-07）：双开区间与两端都有。
	from := time.Date(2024, 7, 4, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 7, 4, 0, 0, 0, 0, time.UTC)
	c, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: subject.ID, PropertyKey: "author",
		Value: knowledge.EntityValue(target.ID), OriginType: knowledge.OriginHuman,
		ValidFrom: &from, ValidTo: &to, Rank: knowledge.RankPreferred,
		Qualifiers: json.RawMessage(`{"pen_name":"项目组"}`), ActorID: actor,
	})
	if err != nil {
		t.Fatalf("带有效时间创建失败: %v", err)
	}
	got, err := svc.GetClaim(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetClaim 失败: %v", err)
	}
	if got.ValidFrom == nil || !got.ValidFrom.Equal(from) || got.ValidTo == nil || !got.ValidTo.Equal(to) {
		t.Fatalf("有效时间回读异常: from=%v to=%v", got.ValidFrom, got.ValidTo)
	}
	if got.Rank != knowledge.RankPreferred {
		t.Fatalf("rank 回读异常: %q", got.Rank)
	}
	// jsonb 回读不保留原始空白，按语义比较。
	var qualifiers map[string]string
	if err := json.Unmarshal(got.QualifiersJSON, &qualifiers); err != nil || qualifiers["pen_name"] != "项目组" {
		t.Fatalf("qualifiers 回读异常: %s", got.QualifiersJSON)
	}
}

func TestCreateClaim_ValidationErrors(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	subject := createEntity(t, svc, "person", "anby", "安比", actor)

	// subject 不存在 / 已 merged。
	_, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: tdb.NewID(t), PropertyKey: "release_date",
		Value: knowledge.DateValue("2024-07-04"), OriginType: knowledge.OriginHuman, ActorID: actor,
	})
	if !errors.Is(err, knowledge.ErrEntityNotFound) {
		t.Fatalf("subject 不存在 err = %v, 期望 ErrEntityNotFound", err)
	}
	merged := createEntity(t, svc, "person", "anby-old", "安比旧", actor)
	target := createEntity(t, svc, "person", "anby-new", "安比新", actor)
	tdb.SetEntityMerged(t, merged.ID, target.ID)
	_, err = svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: merged.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2024-07-04"), OriginType: knowledge.OriginHuman, ActorID: actor,
	})
	if !errors.Is(err, knowledge.ErrEntityMerged) {
		t.Fatalf("merged subject err = %v, 期望 ErrEntityMerged", err)
	}

	// property 不存在。
	_, err = svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: subject.ID, PropertyKey: "not-a-prop",
		Value: knowledge.StringValue("x"), OriginType: knowledge.OriginHuman, ActorID: actor,
	})
	if !errors.Is(err, knowledge.ErrPropertyNotFound) {
		t.Fatalf("property 不存在 err = %v, 期望 ErrPropertyNotFound", err)
	}

	// 值形态不符 / rank / origin / 有效时间 / qualifiers。
	base := knowledge.CreateClaimParams{
		SubjectEntityID: subject.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2024-07-04"), OriginType: knowledge.OriginHuman, ActorID: actor,
	}
	bad := base
	bad.Value = knowledge.StringValue("不是日期")
	if _, err := svc.CreateClaim(ctx, bad); !errors.Is(err, knowledge.ErrInvalidClaimValue) {
		t.Fatalf("值形态 err = %v, 期望 ErrInvalidClaimValue", err)
	}
	bad = base
	bad.Rank = "top"
	if _, err := svc.CreateClaim(ctx, bad); !errors.Is(err, knowledge.ErrInvalidRank) {
		t.Fatalf("rank err = %v, 期望 ErrInvalidRank", err)
	}
	bad = base
	bad.OriginType = "robot"
	if _, err := svc.CreateClaim(ctx, bad); !errors.Is(err, knowledge.ErrInvalidOriginType) {
		t.Fatalf("origin err = %v, 期望 ErrInvalidOriginType", err)
	}
	bad = base
	now := time.Now()
	bad.ValidFrom, bad.ValidTo = &now, &now // valid_to == valid_from 拒绝
	if _, err := svc.CreateClaim(ctx, bad); !errors.Is(err, knowledge.ErrInvalidValidTime) {
		t.Fatalf("valid_to == valid_from err = %v, 期望 ErrInvalidValidTime", err)
	}
	bad = base
	bad.Qualifiers = json.RawMessage(`["not-object"]`)
	if _, err := svc.CreateClaim(ctx, bad); !errors.Is(err, knowledge.ErrInvalidClaimValue) {
		t.Fatalf("qualifiers 非 object err = %v, 期望 ErrInvalidClaimValue", err)
	}

	// ai actor 准入拒绝（与 page/knowledge Entity 的 M5 前立场一致）。
	ai := tdb.MakeActor(t, "ai", "extractor")
	if _, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: subject.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2024-07-04"), OriginType: knowledge.OriginAI, ActorID: ai,
	}); !errors.Is(err, page.ErrActorNotAllowed) {
		t.Fatalf("ai actor err = %v, 期望 page.ErrActorNotAllowed", err)
	}
}

// TestCreateClaim_TypeConstraints subject_type/target_type 列约束与 schema_json 子集约束。
func TestCreateClaim_TypeConstraints(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	person := createEntity(t, svc, "person", "anby", "安比", actor)
	place := createEntity(t, svc, "place", "new-eridu", "新艾利都", actor)
	work := createEntity(t, svc, "work", "zzz", "绝区零", actor)

	// 列约束：subject 必须是 person。
	personID := testkit.EntityTypePersonID
	tdb.MakeProperty(t, "birth_place", "entity", false, &personID, nil, "")
	if _, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: place.ID, PropertyKey: "birth_place",
		Value: knowledge.EntityValue(place.ID), OriginType: knowledge.OriginHuman, ActorID: actor,
	}); !errors.Is(err, knowledge.ErrSubjectTypeMismatch) {
		t.Fatalf("subject 类型不符 err = %v, 期望 ErrSubjectTypeMismatch", err)
	}

	// 列约束：entity 值 target 必须是 work。
	workID := testkit.EntityTypeWorkID
	tdb.MakeProperty(t, "appears_in", "entity", true, nil, &workID, "")
	if _, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: person.ID, PropertyKey: "appears_in",
		Value: knowledge.EntityValue(place.ID), OriginType: knowledge.OriginHuman, ActorID: actor,
	}); !errors.Is(err, knowledge.ErrTargetTypeMismatch) {
		t.Fatalf("target 类型不符 err = %v, 期望 ErrTargetTypeMismatch", err)
	}
	if _, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: person.ID, PropertyKey: "appears_in",
		Value: knowledge.EntityValue(work.ID), OriginType: knowledge.OriginHuman, ActorID: actor,
	}); err != nil {
		t.Fatalf("类型匹配应通过: %v", err)
	}

	// target 不存在 / 已 merged。
	if _, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: person.ID, PropertyKey: "appears_in",
		Value: knowledge.EntityValue(tdb.NewID(t)), OriginType: knowledge.OriginHuman, ActorID: actor,
	}); !errors.Is(err, knowledge.ErrEntityNotFound) {
		t.Fatalf("target 不存在 err = %v, 期望 ErrEntityNotFound", err)
	}
	mergedWork := createEntity(t, svc, "work", "zzz-old", "绝区零旧", actor)
	tdb.SetEntityMerged(t, mergedWork.ID, work.ID)
	if _, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: person.ID, PropertyKey: "appears_in",
		Value: knowledge.EntityValue(mergedWork.ID), OriginType: knowledge.OriginHuman, ActorID: actor,
	}); !errors.Is(err, knowledge.ErrEntityMerged) {
		t.Fatalf("merged target err = %v, 期望 ErrEntityMerged", err)
	}

	// schema_json 子集约束（type_key 形态，000004 种子注释的服务层约定）。
	characterID := testkit.EntityTypeCharacterID
	tdb.MakeProperty(t, "portrayed_by", "entity", true, &characterID, nil, `{"target_type":"person"}`)
	// subject 列约束不满足（person 不是 character）。
	if _, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: person.ID, PropertyKey: "portrayed_by",
		Value: knowledge.EntityValue(person.ID), OriginType: knowledge.OriginHuman, ActorID: actor,
	}); !errors.Is(err, knowledge.ErrSubjectTypeMismatch) {
		t.Fatalf("列约束 err = %v, 期望 ErrSubjectTypeMismatch", err)
	}
	// schema target_type 不满足（target 是 work）。
	character := createEntity(t, svc, "character", "anby-char", "安比(角色)", actor)
	if _, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: character.ID, PropertyKey: "portrayed_by",
		Value: knowledge.EntityValue(work.ID), OriginType: knowledge.OriginHuman, ActorID: actor,
	}); !errors.Is(err, knowledge.ErrTargetTypeMismatch) {
		t.Fatalf("schema target_type err = %v, 期望 ErrTargetTypeMismatch", err)
	}
	// 双约束均满足。
	if _, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: character.ID, PropertyKey: "portrayed_by",
		Value: knowledge.EntityValue(person.ID), OriginType: knowledge.OriginHuman, ActorID: actor,
	}); err != nil {
		t.Fatalf("列+schema 双约束满足应通过: %v", err)
	}
}

// TestCreateClaim_SingleValued 单值 property：已有 published 拒绝创建（提示 Supersede）；
// 仅 proposed 不阻塞；多值 property 自由创建。
func TestCreateClaim_SingleValued(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	subject := createEntity(t, svc, "work", "zzz", "绝区零", actor)
	target := createEntity(t, svc, "organization", "mihoyo", "米哈游", actor)

	createClaim(t, svc, subject.ID, "release_date", knowledge.DateValue("2024-07-04"), actor)

	// 已有 published：拒绝。
	_, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: subject.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2024-07-05"), OriginType: knowledge.OriginHuman, ActorID: actor,
	})
	if !errors.Is(err, knowledge.ErrClaimNotMultivalued) {
		t.Fatalf("err = %v, 期望 ErrClaimNotMultivalued", err)
	}

	// 多值 property：无限制。
	createClaim(t, svc, subject.ID, "developer", knowledge.EntityValue(target.ID), actor)
	createClaim(t, svc, subject.ID, "developer", knowledge.EntityValue(target.ID), actor)

	// 单值 property 仅有 proposed（ai 来源）：不阻塞 published 创建。
	tdb.MakeProperty(t, "tagline", "string", false, nil, nil, "")
	if _, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: subject.ID, PropertyKey: "tagline",
		Value: knowledge.StringValue("AI 草稿"), OriginType: knowledge.OriginAI, ActorID: actor,
	}); err != nil {
		t.Fatalf("ai 来源创建失败: %v", err)
	}
	if _, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: subject.ID, PropertyKey: "tagline",
		Value: knowledge.StringValue("人工定稿"), OriginType: knowledge.OriginHuman, ActorID: actor,
	}); err != nil {
		t.Fatalf("proposed 不应阻塞 published 创建: %v", err)
	}
}

// TestPublishClaim_SingleValuedGuard proposed→published 路径同样守单值不变量：
// 单值 property 已有 published 时，发布另一条 proposed 拒绝。
// 正常服务路径下 proposed 与 published 不会并存（CreateClaim 已拦截），
// 这里用直接 SQL 搭建并存场景，验证守卫是真实有效的兜底。
func TestPublishClaim_SingleValuedGuard(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	subject := createEntity(t, svc, "work", "zzz", "绝区零", actor)

	createClaim(t, svc, subject.ID, "release_date", knowledge.DateValue("2024-07-04"), actor)

	// 绕过服务层直接插入一条 proposed claim（CreateClaim 会拦截此场景）。
	proposedID := tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO claim (id, subject_entity_id, property_id, value_type, value_json,
			status, origin_type, created_by)
		VALUES ($1, $2, $3, 'date', '"2024-07-05"'::jsonb, 'proposed', 'ai', $4)`,
		proposedID, subject.ID, testkit.PropertyReleaseDateID, actor); err != nil {
		t.Fatalf("直接插入 proposed claim 失败: %v", err)
	}

	if _, err := svc.PublishClaim(ctx, proposedID); !errors.Is(err, knowledge.ErrClaimNotMultivalued) {
		t.Fatalf("err = %v, 期望 ErrClaimNotMultivalued", err)
	}
	// 被拒绝后仍是 proposed，可正常 reject。
	got, err := svc.GetClaim(ctx, proposedID)
	if err != nil || got.Status != knowledge.ClaimStatusProposed {
		t.Fatalf("proposed 状态异常: got=%+v err=%v", got, err)
	}
	if _, err := svc.RejectClaim(ctx, proposedID); err != nil {
		t.Fatalf("RejectClaim 失败: %v", err)
	}
}

// TestCreateClaim_ConcurrentSingleValued 并发创建单值 property：subject 行锁序列化，恰一成功。
func TestCreateClaim_ConcurrentSingleValued(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	subject := createEntity(t, svc, "work", "zzz", "绝区零", actor)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	dates := []string{"2024-07-04", "2024-07-05"}
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.CreateClaim(ctx, knowledge.CreateClaimParams{
				SubjectEntityID: subject.ID, PropertyKey: "release_date",
				Value: knowledge.DateValue(dates[i]), OriginType: knowledge.OriginHuman, ActorID: actor,
			})
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, knowledge.ErrClaimNotMultivalued) {
			t.Fatalf("失败方 err = %v, 期望 ErrClaimNotMultivalued", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("成功数 = %d, 期望恰好 1", succeeded)
	}
}

// TestSupersedeClaim_Chain Supersede 链：旧状态/指针/新值、链上再 supersede、
// 已 superseded 拒绝、跨 subject/property 断言拒绝、非 published 拒绝。
func TestSupersedeClaim_Chain(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	subject := createEntity(t, svc, "work", "zzz", "绝区零", actor)
	other := createEntity(t, svc, "work", "other-game", "另一作品", actor)

	old := createClaim(t, svc, subject.ID, "release_date", knowledge.DateValue("2024-07-04"), actor)

	// 正常 supersede：旧置 superseded 并指向新 claim，新 claim 带新值且 published。
	v2, err := svc.SupersedeClaim(ctx, knowledge.SupersedeClaimParams{
		ClaimID: old.ID, SubjectEntityID: subject.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2024-07-05"), ActorID: actor,
	})
	if err != nil {
		t.Fatalf("SupersedeClaim 失败: %v", err)
	}
	if string(v2.ValueJSON) != `"2024-07-05"` || v2.Status != knowledge.ClaimStatusPublished {
		t.Fatalf("新 claim 异常: value=%s status=%q", v2.ValueJSON, v2.Status)
	}
	if v2.OriginType != knowledge.OriginHuman {
		t.Fatalf("新 claim origin = %q, 期望继承旧 claim 的 human", v2.OriginType)
	}
	oldGot, err := svc.GetClaim(ctx, old.ID)
	if err != nil {
		t.Fatalf("GetClaim 失败: %v", err)
	}
	if oldGot.Status != knowledge.ClaimStatusSuperseded || oldGot.SupersededBy == nil || *oldGot.SupersededBy != v2.ID {
		t.Fatalf("旧 claim 异常: status=%q superseded_by=%v", oldGot.Status, oldGot.SupersededBy)
	}

	// 链上再 supersede（对链头 v2 合法）。
	v3, err := svc.SupersedeClaim(ctx, knowledge.SupersedeClaimParams{
		ClaimID: v2.ID, SubjectEntityID: subject.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2024-07-06"), ActorID: actor,
	})
	if err != nil {
		t.Fatalf("链上二次 supersede 失败: %v", err)
	}
	v2Got, _ := svc.GetClaim(ctx, v2.ID)
	if v2Got.Status != knowledge.ClaimStatusSuperseded || *v2Got.SupersededBy != v3.ID {
		t.Fatalf("链中间节点异常: status=%q superseded_by=%v", v2Got.Status, v2Got.SupersededBy)
	}

	// 已 superseded 的再 supersede：拒绝。
	if _, err := svc.SupersedeClaim(ctx, knowledge.SupersedeClaimParams{
		ClaimID: old.ID, SubjectEntityID: subject.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2024-07-07"), ActorID: actor,
	}); !errors.Is(err, knowledge.ErrInvalidClaimTransition) {
		t.Fatalf("已 superseded err = %v, 期望 ErrInvalidClaimTransition", err)
	}

	// 跨 subject / property 断言拒绝。
	if _, err := svc.SupersedeClaim(ctx, knowledge.SupersedeClaimParams{
		ClaimID: v3.ID, SubjectEntityID: other.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2024-07-07"), ActorID: actor,
	}); !errors.Is(err, knowledge.ErrClaimSubjectMismatch) {
		t.Fatalf("跨 subject err = %v, 期望 ErrClaimSubjectMismatch", err)
	}
	if _, err := svc.SupersedeClaim(ctx, knowledge.SupersedeClaimParams{
		ClaimID: v3.ID, SubjectEntityID: subject.ID, PropertyKey: "instance_of",
		Value: knowledge.DateValue("2024-07-07"), ActorID: actor,
	}); !errors.Is(err, knowledge.ErrClaimSubjectMismatch) {
		t.Fatalf("跨 property err = %v, 期望 ErrClaimSubjectMismatch", err)
	}

	// 非 published（proposed）不可 supersede；claim 不存在。
	tdb.MakeProperty(t, "tagline", "string", true, nil, nil, "")
	proposed, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: subject.ID, PropertyKey: "tagline",
		Value: knowledge.StringValue("草稿"), OriginType: knowledge.OriginAI, ActorID: actor,
	})
	if err != nil {
		t.Fatalf("创建 proposed claim 失败: %v", err)
	}
	if _, err := svc.SupersedeClaim(ctx, knowledge.SupersedeClaimParams{
		ClaimID: proposed.ID, SubjectEntityID: subject.ID, PropertyKey: "tagline",
		Value: knowledge.StringValue("x"), ActorID: actor,
	}); !errors.Is(err, knowledge.ErrInvalidClaimTransition) {
		t.Fatalf("proposed supersede err = %v, 期望 ErrInvalidClaimTransition", err)
	}
	if _, err := svc.SupersedeClaim(ctx, knowledge.SupersedeClaimParams{
		ClaimID: tdb.NewID(t), SubjectEntityID: subject.ID, PropertyKey: "tagline",
		Value: knowledge.StringValue("x"), ActorID: actor,
	}); !errors.Is(err, knowledge.ErrClaimNotFound) {
		t.Fatalf("claim 不存在 err = %v, 期望 ErrClaimNotFound", err)
	}
}

// TestSupersedeClaim_Concurrent 并发双 supersede：旧 claim 行锁序列化，恰一成功。
func TestSupersedeClaim_Concurrent(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	subject := createEntity(t, svc, "work", "zzz", "绝区零", actor)
	old := createClaim(t, svc, subject.ID, "release_date", knowledge.DateValue("2024-07-04"), actor)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	newIDs := make([]uuid.UUID, 2)
	dates := []string{"2024-07-05", "2024-07-06"}
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := svc.SupersedeClaim(ctx, knowledge.SupersedeClaimParams{
				ClaimID: old.ID, SubjectEntityID: subject.ID, PropertyKey: "release_date",
				Value: knowledge.DateValue(dates[i]), ActorID: actor,
			})
			errs[i] = err
			if c != nil {
				newIDs[i] = c.ID
			}
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for i, err := range errs {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, knowledge.ErrInvalidClaimTransition) {
			t.Fatalf("失败方 err = %v, 期望 ErrInvalidClaimTransition", err)
		} else if newIDs[i] != uuid.Nil {
			t.Fatalf("失败方不应产生新 claim")
		}
	}
	if succeeded != 1 {
		t.Fatalf("成功数 = %d, 期望恰好 1", succeeded)
	}

	// 旧 claim 只被 supersede 一次，指针指向胜者的 claim。
	oldGot, err := svc.GetClaim(ctx, old.ID)
	if err != nil {
		t.Fatalf("GetClaim 失败: %v", err)
	}
	if oldGot.Status != knowledge.ClaimStatusSuperseded {
		t.Fatalf("旧 claim status = %q", oldGot.Status)
	}
	pointed := false
	for _, id := range newIDs {
		if id != uuid.Nil && *oldGot.SupersededBy == id {
			pointed = true
		}
	}
	if !pointed {
		t.Fatalf("superseded_by=%v 未指向任一并发新 claim", oldGot.SupersededBy)
	}
}

// TestClaimStatusTransitions 状态机关键转换（全矩阵见单元测试 TestClaimStatusTransitionMatrix）。
func TestClaimStatusTransitions(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	subject := createEntity(t, svc, "work", "zzz", "绝区零", actor)

	newProposed := func(key string) *knowledge.Claim {
		t.Helper()
		c, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
			SubjectEntityID: subject.ID, PropertyKey: key,
			Value: knowledge.StringValue("草稿"), OriginType: knowledge.OriginAI, ActorID: actor,
		})
		if err != nil {
			t.Fatalf("创建 proposed claim 失败: %v", err)
		}
		return c
	}
	tdb.MakeProperty(t, "p1", "string", true, nil, nil, "")
	tdb.MakeProperty(t, "p2", "string", true, nil, nil, "")

	// proposed→published→deprecated，终态再流转拒绝。
	c1 := newProposed("p1")
	if got, err := svc.PublishClaim(ctx, c1.ID); err != nil || got.Status != knowledge.ClaimStatusPublished {
		t.Fatalf("PublishClaim: got=%v err=%v", got, err)
	}
	if _, err := svc.PublishClaim(ctx, c1.ID); !errors.Is(err, knowledge.ErrInvalidClaimTransition) {
		t.Fatalf("重复 publish err = %v, 期望 ErrInvalidClaimTransition", err)
	}
	if _, err := svc.RejectClaim(ctx, c1.ID); !errors.Is(err, knowledge.ErrInvalidClaimTransition) {
		t.Fatalf("published→rejected err = %v, 期望 ErrInvalidClaimTransition", err)
	}
	if got, err := svc.DeprecateClaim(ctx, c1.ID); err != nil || got.Status != knowledge.ClaimStatusDeprecated {
		t.Fatalf("DeprecateClaim: got=%v err=%v", got, err)
	}
	if _, err := svc.DeprecateClaim(ctx, c1.ID); !errors.Is(err, knowledge.ErrInvalidClaimTransition) {
		t.Fatalf("deprecated 再流转 err = %v, 期望 ErrInvalidClaimTransition", err)
	}

	// proposed→rejected，终态再流转拒绝。
	c2 := newProposed("p2")
	if got, err := svc.RejectClaim(ctx, c2.ID); err != nil || got.Status != knowledge.ClaimStatusRejected {
		t.Fatalf("RejectClaim: got=%v err=%v", got, err)
	}
	if _, err := svc.PublishClaim(ctx, c2.ID); !errors.Is(err, knowledge.ErrInvalidClaimTransition) {
		t.Fatalf("rejected→published err = %v, 期望 ErrInvalidClaimTransition", err)
	}

	// proposed 不能直接 deprecated（只能经 published）。
	c3 := newProposed("p1")
	if _, err := svc.DeprecateClaim(ctx, c3.ID); !errors.Is(err, knowledge.ErrInvalidClaimTransition) {
		t.Fatalf("proposed→deprecated err = %v, 期望 ErrInvalidClaimTransition", err)
	}

	// claim 不存在。
	if _, err := svc.PublishClaim(ctx, tdb.NewID(t)); !errors.Is(err, knowledge.ErrClaimNotFound) {
		t.Fatalf("claim 不存在 err = %v, 期望 ErrClaimNotFound", err)
	}
}

// TestUpdateVerificationStatus 验证状态权限与持久化（INV-07）。
func TestUpdateVerificationStatus(t *testing.T) {
	tdb, svc := setup(t)
	human := tdb.MakeActor(t, "human", "alice")
	ai := tdb.MakeActor(t, "ai", "checker")
	bot := tdb.MakeActor(t, "bot", "importer")
	ctx := context.Background()
	subject := createEntity(t, svc, "work", "zzz", "绝区零", human)
	claim := createClaim(t, svc, subject.ID, "release_date", knowledge.DateValue("2024-07-04"), human)

	set := func(actor uuid.UUID, status string) error {
		return svc.UpdateVerificationStatus(ctx, knowledge.UpdateVerificationStatusParams{
			ClaimID: claim.ID, Status: status, ActorID: actor,
		})
	}

	// human 可置 human_verified / disputed。
	if err := set(human, knowledge.VerificationHumanVerified); err != nil {
		t.Fatalf("human 置 human_verified 失败: %v", err)
	}
	got, err := svc.GetClaim(ctx, claim.ID)
	if err != nil || got.VerificationStatus != knowledge.VerificationHumanVerified {
		t.Fatalf("验证状态未持久化: got=%+v err=%v", got, err)
	}
	if err := set(human, knowledge.VerificationDisputed); err != nil {
		t.Fatalf("human 置 disputed 失败: %v", err)
	}

	// ai 只能置 ai_checked。
	if err := set(ai, knowledge.VerificationAIChecked); err != nil {
		t.Fatalf("ai 置 ai_checked 失败: %v", err)
	}
	if err := set(ai, knowledge.VerificationHumanVerified); !errors.Is(err, knowledge.ErrVerificationForbidden) {
		t.Fatalf("ai 置 human_verified err = %v, 期望 ErrVerificationForbidden", err)
	}

	// bot 无权；非法状态；未知 actor；未知 claim。
	if err := set(bot, knowledge.VerificationAIChecked); !errors.Is(err, knowledge.ErrVerificationForbidden) {
		t.Fatalf("bot err = %v, 期望 ErrVerificationForbidden", err)
	}
	if err := set(human, "verified"); !errors.Is(err, knowledge.ErrInvalidVerificationStatus) {
		t.Fatalf("非法状态 err = %v, 期望 ErrInvalidVerificationStatus", err)
	}
	if err := set(tdb.NewID(t), knowledge.VerificationAIChecked); !errors.Is(err, page.ErrInvalidActor) {
		t.Fatalf("未知 actor err = %v, 期望 page.ErrInvalidActor", err)
	}
	if err := svc.UpdateVerificationStatus(ctx, knowledge.UpdateVerificationStatusParams{
		ClaimID: tdb.NewID(t), Status: knowledge.VerificationAIChecked, ActorID: human,
	}); !errors.Is(err, knowledge.ErrClaimNotFound) {
		t.Fatalf("未知 claim err = %v, 期望 ErrClaimNotFound", err)
	}
}

// TestAddClaimSource claim 绑定 citation 位（INV-07）：幂等拒绝与终态拒绝。
// 000007（M4-T04）已给 claim_source.citation_id 补外键；M4-T05 起服务层经注入的
// CitationChecker 做 citation 存在性校验（见 TestAddClaimSource_CitationExists）。
func TestAddClaimSource(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	subject := createEntity(t, svc, "work", "zzz", "绝区零", actor)
	claim := createClaim(t, svc, subject.ID, "release_date", knowledge.DateValue("2024-07-04"), actor)

	add := func(claimID, citationID uuid.UUID, supportType string) error {
		_, err := svc.AddClaimSource(ctx, knowledge.AddClaimSourceParams{
			ClaimID: claimID, CitationID: citationID, SupportType: supportType,
		})
		return err
	}

	// 三种 support_type 各绑定一个 citation。
	c1, c2, c3 := tdb.MakeCitation(t, actor), tdb.MakeCitation(t, actor), tdb.MakeCitation(t, actor)
	for _, st := range []struct {
		id  uuid.UUID
		typ string
	}{{c1, knowledge.SupportTypeSupports}, {c2, knowledge.SupportTypeContradicts}, {c3, knowledge.SupportTypeContext}} {
		if err := add(claim.ID, st.id, st.typ); err != nil {
			t.Fatalf("AddClaimSource(%s) 失败: %v", st.typ, err)
		}
	}
	sources, err := svc.ListClaimSources(ctx, claim.ID)
	if err != nil {
		t.Fatalf("ListClaimSources 失败: %v", err)
	}
	if len(sources) != 3 {
		t.Fatalf("来源数 = %d, 期望 3", len(sources))
	}

	// 重复添加（同 claim+citation，即使换 support_type）：幂等拒绝。
	if err := add(claim.ID, c1, knowledge.SupportTypeSupports); !errors.Is(err, knowledge.ErrClaimSourceExists) {
		t.Fatalf("重复添加 err = %v, 期望 ErrClaimSourceExists", err)
	}
	if err := add(claim.ID, c1, knowledge.SupportTypeContext); !errors.Is(err, knowledge.ErrClaimSourceExists) {
		t.Fatalf("换 support_type 重复 err = %v, 期望 ErrClaimSourceExists", err)
	}

	// 非法 support_type / Nil citation / claim 不存在。
	if err := add(claim.ID, tdb.NewID(t), "cites"); !errors.Is(err, knowledge.ErrInvalidSupportType) {
		t.Fatalf("support_type err = %v, 期望 ErrInvalidSupportType", err)
	}
	if err := add(claim.ID, uuid.Nil, knowledge.SupportTypeSupports); !errors.Is(err, knowledge.ErrInvalidClaimValue) {
		t.Fatalf("Nil citation err = %v, 期望 ErrInvalidClaimValue", err)
	}
	if err := add(tdb.NewID(t), tdb.NewID(t), knowledge.SupportTypeSupports); !errors.Is(err, knowledge.ErrClaimNotFound) {
		t.Fatalf("claim 不存在 err = %v, 期望 ErrClaimNotFound", err)
	}

	// 终态拒绝：deprecated 与 superseded。
	if _, err := svc.DeprecateClaim(ctx, claim.ID); err != nil {
		t.Fatalf("DeprecateClaim 失败: %v", err)
	}
	if err := add(claim.ID, tdb.NewID(t), knowledge.SupportTypeSupports); !errors.Is(err, knowledge.ErrClaimTerminal) {
		t.Fatalf("deprecated claim err = %v, 期望 ErrClaimTerminal", err)
	}
	other := createClaim(t, svc, subject.ID, "instance_of", knowledge.EntityValue(createEntity(t, svc, "concept", "arpg", "ARPG", actor).ID), actor)
	if _, err := svc.SupersedeClaim(ctx, knowledge.SupersedeClaimParams{
		ClaimID: other.ID, SubjectEntityID: subject.ID, PropertyKey: "instance_of",
		Value: knowledge.EntityValue(createEntity(t, svc, "concept", "action-rpg", "动作RPG", actor).ID), ActorID: actor,
	}); err != nil {
		t.Fatalf("SupersedeClaim 失败: %v", err)
	}
	if err := add(other.ID, tdb.NewID(t), knowledge.SupportTypeSupports); !errors.Is(err, knowledge.ErrClaimTerminal) {
		t.Fatalf("superseded claim err = %v, 期望 ErrClaimTerminal", err)
	}

	// rejected 非终态拒绝对象：仍可补来源（留待人工复核证据链）。
	tdb.MakeProperty(t, "tagline", "string", true, nil, nil, "")
	rejected, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: subject.ID, PropertyKey: "tagline",
		Value: knowledge.StringValue("草稿"), OriginType: knowledge.OriginAI, ActorID: actor,
	})
	if err != nil {
		t.Fatalf("创建 proposed claim 失败: %v", err)
	}
	if _, err := svc.RejectClaim(ctx, rejected.ID); err != nil {
		t.Fatalf("RejectClaim 失败: %v", err)
	}
	if err := add(rejected.ID, tdb.MakeCitation(t, actor), knowledge.SupportTypeContext); err != nil {
		t.Fatalf("rejected claim 添加来源应允许: %v", err)
	}
}

// TestAddClaimSource_CitationExists citation 存在性校验（INV-07 完整链路，M4-T05）：
// 不存在的 citation 返回 ErrCitationNotFound；存在的 citation 绑定成功。
func TestAddClaimSource_CitationExists(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	subject := createEntity(t, svc, "work", "zzz", "绝区零", actor)
	claim := createClaim(t, svc, subject.ID, "release_date", knowledge.DateValue("2024-07-04"), actor)

	if _, err := svc.AddClaimSource(ctx, knowledge.AddClaimSourceParams{
		ClaimID: claim.ID, CitationID: tdb.NewID(t), SupportType: knowledge.SupportTypeSupports,
	}); !errors.Is(err, knowledge.ErrCitationNotFound) {
		t.Fatalf("不存在 citation err = %v, 期望 ErrCitationNotFound", err)
	}

	citationID := tdb.MakeCitation(t, actor)
	if _, err := svc.AddClaimSource(ctx, knowledge.AddClaimSourceParams{
		ClaimID: claim.ID, CitationID: citationID, SupportType: knowledge.SupportTypeSupports,
	}); err != nil {
		t.Fatalf("存在 citation 绑定失败: %v", err)
	}
}

// TestListClaims 过滤查询：property/status/verification_status 组合。
func TestListClaims(t *testing.T) {
	tdb, svc := setup(t)
	actor := tdb.MakeActor(t, "human", "alice")
	ctx := context.Background()
	subject := createEntity(t, svc, "work", "zzz", "绝区零", actor)
	other := createEntity(t, svc, "work", "other", "另一作品", actor)
	target := createEntity(t, svc, "organization", "mihoyo", "米哈游", actor)

	published := createClaim(t, svc, subject.ID, "release_date", knowledge.DateValue("2024-07-04"), actor)
	proposed, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: subject.ID, PropertyKey: "developer",
		Value: knowledge.EntityValue(target.ID), OriginType: knowledge.OriginAI, ActorID: actor,
	})
	if err != nil {
		t.Fatalf("创建 proposed claim 失败: %v", err)
	}
	createClaim(t, svc, other.ID, "release_date", knowledge.DateValue("2025-01-01"), actor)
	if err := svc.UpdateVerificationStatus(ctx, knowledge.UpdateVerificationStatusParams{
		ClaimID: published.ID, Status: knowledge.VerificationHumanVerified, ActorID: actor,
	}); err != nil {
		t.Fatalf("UpdateVerificationStatus 失败: %v", err)
	}

	list := func(params knowledge.ListClaimsParams) []knowledge.Claim {
		t.Helper()
		claims, err := svc.ListClaims(ctx, params)
		if err != nil {
			t.Fatalf("ListClaims(%+v) 失败: %v", params, err)
		}
		return claims
	}

	// 全量（仅本 subject）。
	if got := list(knowledge.ListClaimsParams{SubjectEntityID: subject.ID}); len(got) != 2 {
		t.Fatalf("全量 = %d, 期望 2", len(got))
	}
	// property 过滤。
	got := list(knowledge.ListClaimsParams{SubjectEntityID: subject.ID, PropertyKey: "release_date"})
	if len(got) != 1 || got[0].ID != published.ID {
		t.Fatalf("property 过滤异常: %+v", got)
	}
	// status 过滤。
	got = list(knowledge.ListClaimsParams{SubjectEntityID: subject.ID, Status: knowledge.ClaimStatusProposed})
	if len(got) != 1 || got[0].ID != proposed.ID {
		t.Fatalf("status 过滤异常: %+v", got)
	}
	// verification_status 过滤。
	got = list(knowledge.ListClaimsParams{SubjectEntityID: subject.ID, VerificationStatus: knowledge.VerificationHumanVerified})
	if len(got) != 1 || got[0].ID != published.ID {
		t.Fatalf("verification 过滤异常: %+v", got)
	}
	// 组合过滤空结果。
	if got := list(knowledge.ListClaimsParams{
		SubjectEntityID: subject.ID, PropertyKey: "release_date", Status: knowledge.ClaimStatusProposed,
	}); len(got) != 0 {
		t.Fatalf("组合过滤应为空: %+v", got)
	}
	// 非法过滤参数。
	if _, err := svc.ListClaims(ctx, knowledge.ListClaimsParams{SubjectEntityID: subject.ID, Status: "live"}); !errors.Is(err, knowledge.ErrInvalidClaimStatus) {
		t.Fatalf("非法 status err = %v, 期望 ErrInvalidClaimStatus", err)
	}
	if _, err := svc.ListClaims(ctx, knowledge.ListClaimsParams{SubjectEntityID: subject.ID, VerificationStatus: "ok"}); !errors.Is(err, knowledge.ErrInvalidVerificationStatus) {
		t.Fatalf("非法 verification err = %v, 期望 ErrInvalidVerificationStatus", err)
	}
	if _, err := svc.ListClaims(ctx, knowledge.ListClaimsParams{SubjectEntityID: subject.ID, PropertyKey: "nope"}); !errors.Is(err, knowledge.ErrPropertyNotFound) {
		t.Fatalf("未知 property err = %v, 期望 ErrPropertyNotFound", err)
	}
}
