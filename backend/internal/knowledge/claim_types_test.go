package knowledge

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"
)

// testProperty 构造最小 Property（normalizeValue 只依赖 ValueType/PropertyKey/SchemaJSON）。
func testProperty(valueType string, schema json.RawMessage) *Property {
	return &Property{PropertyKey: "p_" + valueType, ValueType: valueType, SchemaJSON: schema}
}

func TestNormalizeValue_ValidShapes(t *testing.T) {
	targetID := uuid.MustParse("00000000-0000-7000-8000-000000000777")
	tests := []struct {
		name       string
		prop       *Property
		value      Value
		wantJSON   string
		wantTarget *uuid.UUID
	}{
		{"string", testProperty(ValueTypeString, nil), StringValue("安比"), `"安比"`, nil},
		{"number 整数", testProperty(ValueTypeNumber, nil), NumberValue(42), `42`, nil},
		{"number 小数", testProperty(ValueTypeNumber, nil), NumberValue(1.5), `1.5`, nil},
		{"date RFC3339", testProperty(ValueTypeDate, nil), DateValue("2024-07-04"), `"2024-07-04"`, nil},
		{"entity", testProperty(ValueTypeEntity, nil), EntityValue(targetID), `{"entity_id":"` + targetID.String() + `"}`, &targetID},
		{"coordinate", testProperty(ValueTypeCoordinate, nil), CoordinateValue(31.23, 121.47), `{"lat":31.23,"lon":121.47}`, nil},
		{"coordinate 边界", testProperty(ValueTypeCoordinate, nil), CoordinateValue(-90, 180), `{"lat":-90,"lon":180}`, nil},
		{"composite 自由 object", testProperty(ValueTypeComposite, nil), CompositeValue(json.RawMessage(`{"a":1,"b":[true]}`)), `{"a":1,"b":[true]}`, nil},
		{
			"composite 满足 schema 子集",
			testProperty(ValueTypeComposite, json.RawMessage(`{"value":{"required":["qty"],"properties":{"qty":{"type":"number"}}}}`)),
			CompositeValue(json.RawMessage(`{"qty":3,"note":"x"}`)),
			`{"qty":3,"note":"x"}`, nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valueJSON, target, err := normalizeValue(tt.prop, tt.value)
			if err != nil {
				t.Fatalf("normalizeValue 出错: %v", err)
			}
			if string(valueJSON) != tt.wantJSON {
				t.Fatalf("value_json = %s, 期望 %s", valueJSON, tt.wantJSON)
			}
			if (target == nil) != (tt.wantTarget == nil) {
				t.Fatalf("target_entity_id = %v, 期望 %v", target, tt.wantTarget)
			}
			if target != nil && *target != *tt.wantTarget {
				t.Fatalf("target_entity_id = %s, 期望 %s", target, tt.wantTarget)
			}
		})
	}
}

func TestNormalizeValue_InvalidShapes(t *testing.T) {
	tests := []struct {
		name  string
		prop  *Property
		value Value
	}{
		{"string 缺失", testProperty(ValueTypeString, nil), Value{}},
		{"string 空串", testProperty(ValueTypeString, nil), StringValue("")},
		{"number 缺失", testProperty(ValueTypeNumber, nil), Value{}},
		{"number NaN", testProperty(ValueTypeNumber, nil), NumberValue(math.NaN())},
		{"number +Inf", testProperty(ValueTypeNumber, nil), NumberValue(math.Inf(1))},
		{"date 缺失", testProperty(ValueTypeDate, nil), Value{}},
		{"date 非 RFC3339", testProperty(ValueTypeDate, nil), DateValue("2024/07/04")},
		{"date 带时间", testProperty(ValueTypeDate, nil), DateValue("2024-07-04T10:00:00Z")},
		{"date 不存在的日期", testProperty(ValueTypeDate, nil), DateValue("2024-13-01")},
		{"entity 缺失", testProperty(ValueTypeEntity, nil), Value{}},
		{"entity Nil", testProperty(ValueTypeEntity, nil), EntityValue(uuid.Nil)},
		{"coordinate 缺失", testProperty(ValueTypeCoordinate, nil), Value{}},
		{"coordinate lat 越界", testProperty(ValueTypeCoordinate, nil), CoordinateValue(91, 0)},
		{"coordinate lon 越界", testProperty(ValueTypeCoordinate, nil), CoordinateValue(0, -181)},
		{"coordinate NaN", testProperty(ValueTypeCoordinate, nil), CoordinateValue(math.NaN(), 0)},
		{"composite 缺失", testProperty(ValueTypeComposite, nil), Value{}},
		{"composite 非 object", testProperty(ValueTypeComposite, nil), CompositeValue(json.RawMessage(`[1,2]`))},
		{"composite null", testProperty(ValueTypeComposite, nil), CompositeValue(json.RawMessage(`null`))},
		{"composite 非法 JSON", testProperty(ValueTypeComposite, nil), CompositeValue(json.RawMessage(`{`))},
		{
			"composite 缺必填键",
			testProperty(ValueTypeComposite, json.RawMessage(`{"value":{"required":["qty"]}}`)),
			CompositeValue(json.RawMessage(`{"note":"x"}`)),
		},
		{
			"composite 键类型不符",
			testProperty(ValueTypeComposite, json.RawMessage(`{"value":{"properties":{"qty":{"type":"number"}}}}`)),
			CompositeValue(json.RawMessage(`{"qty":"三"}`)),
		},
		{"未知 value_type", testProperty("wat", nil), StringValue("x")},
		{"值类型与提供字段不符", testProperty(ValueTypeString, nil), NumberValue(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := normalizeValue(tt.prop, tt.value)
			if !errors.Is(err, ErrInvalidClaimValue) {
				t.Fatalf("err = %v, 期望 ErrInvalidClaimValue", err)
			}
		})
	}
}

// TestClaimStatusTransitionMatrix 业务状态机全矩阵（设计 §6.5）：
// proposed→published/rejected；published→deprecated/superseded；其余全部非法。
func TestClaimStatusTransitionMatrix(t *testing.T) {
	statuses := []string{
		ClaimStatusProposed, ClaimStatusPublished, ClaimStatusRejected,
		ClaimStatusSuperseded, ClaimStatusDeprecated,
	}
	allowed := map[string]map[string]bool{
		ClaimStatusProposed:  {ClaimStatusPublished: true, ClaimStatusRejected: true},
		ClaimStatusPublished: {ClaimStatusDeprecated: true, ClaimStatusSuperseded: true},
	}
	for _, from := range statuses {
		for _, to := range statuses {
			want := allowed[from][to]
			if got := canTransitionClaim(from, to); got != want {
				t.Fatalf("canTransitionClaim(%q, %q) = %v, 期望 %v", from, to, got, want)
			}
		}
	}
	// 终态没有任何出边。
	for _, terminal := range []string{ClaimStatusRejected, ClaimStatusSuperseded, ClaimStatusDeprecated} {
		if len(claimStatusTransitions[terminal]) != 0 {
			t.Fatalf("终态 %q 不应有出边: %v", terminal, claimStatusTransitions[terminal])
		}
	}
}

// TestCheckVerificationPermission 验证状态权限矩阵（防御性）：
// human 全四种可置；ai 只能 ai_checked；bot/system 无权；非法状态先报 ErrInvalidVerificationStatus。
func TestCheckVerificationPermission(t *testing.T) {
	statuses := []string{
		VerificationUnverified, VerificationAIChecked,
		VerificationHumanVerified, VerificationDisputed,
	}
	for _, st := range statuses {
		if err := checkVerificationPermission("human", st); err != nil {
			t.Fatalf("human 置 %q 应允许: %v", st, err)
		}
	}
	if err := checkVerificationPermission("ai", VerificationAIChecked); err != nil {
		t.Fatalf("ai 置 ai_checked 应允许: %v", err)
	}
	for _, st := range []string{VerificationUnverified, VerificationHumanVerified, VerificationDisputed} {
		if err := checkVerificationPermission("ai", st); !errors.Is(err, ErrVerificationForbidden) {
			t.Fatalf("ai 置 %q err = %v, 期望 ErrVerificationForbidden", st, err)
		}
	}
	for _, actorType := range []string{"bot", "system", "anonymous"} {
		if err := checkVerificationPermission(actorType, VerificationAIChecked); !errors.Is(err, ErrVerificationForbidden) {
			t.Fatalf("%s err = %v, 期望 ErrVerificationForbidden", actorType, err)
		}
	}
	if err := checkVerificationPermission("human", "verified"); !errors.Is(err, ErrInvalidVerificationStatus) {
		t.Fatalf("非法状态 err = %v, 期望 ErrInvalidVerificationStatus", err)
	}
}

func TestIsValidSupportType(t *testing.T) {
	for _, st := range []string{SupportTypeSupports, SupportTypeContradicts, SupportTypeContext} {
		if !isValidSupportType(st) {
			t.Fatalf("support_type %q 应合法", st)
		}
	}
	for _, st := range []string{"", "cites", "SUPPORTS"} {
		if isValidSupportType(st) {
			t.Fatalf("support_type %q 应非法", st)
		}
	}
}

func TestParsePropertySchema(t *testing.T) {
	// 空 / {} 均视为无附加约束。
	for _, raw := range []json.RawMessage{nil, {}, []byte(`{}`)} {
		s, err := parsePropertySchema(raw)
		if err != nil {
			t.Fatalf("parsePropertySchema(%s) 出错: %v", raw, err)
		}
		if s.SubjectType != "" || s.TargetType != "" || s.Value != nil {
			t.Fatalf("parsePropertySchema(%s) 应为零值 schema: %+v", raw, s)
		}
	}
	s, err := parsePropertySchema(json.RawMessage(`{"subject_type":"person","target_type":"work","value":{"required":["x"]}}`))
	if err != nil {
		t.Fatalf("parsePropertySchema 出错: %v", err)
	}
	if s.SubjectType != "person" || s.TargetType != "work" || s.Value == nil || s.Value.Required[0] != "x" {
		t.Fatalf("schema 解析异常: %+v", s)
	}
	if _, err := parsePropertySchema(json.RawMessage(`{`)); !errors.Is(err, ErrInvalidClaimValue) {
		t.Fatalf("非法 JSON err = %v, 期望 ErrInvalidClaimValue", err)
	}
}
