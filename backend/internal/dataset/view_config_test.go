package dataset

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func testFields() map[string]fieldDefinition {
	return map[string]fieldDefinition{
		"title":  {Type: "string", Title: "Title", Required: true},
		"status": {Type: "string", Title: "Status"},
		"score":  {Type: "number", Title: "Score"},
		"due":    {Type: "string", Format: "date", Title: "Due"},
	}
}

func TestParseViewConfigAdvancedQuery(t *testing.T) {
	raw := json.RawMessage(`{
		"columns":["title","status","score"],
		"filter":{"logic":"and","conditions":[
			{"field":"status","operator":"contains","value":"open"},
			{"logic":"or","conditions":[
				{"field":"score","operator":"gte","value":10},
				{"field":"due","operator":"before","value":"2027-01-01"}
			]}
		]},
		"sorts":[
			{"field":"score","direction":"desc"},
			{"field":"title","direction":"asc"}
		],
		"groups":[
			{"field":"status","display":"collapsed","sort_direction":"desc",
			 "options":["open","done"]},
			{"field":"due","hide_empty":true}
		],
		"title_field":"title"
	}`)
	config, canonical, err := parseViewConfig(raw, testFields(), ViewBoard)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Sorts) != 2 || len(config.Groups) != 2 {
		t.Fatalf("unexpected normalized config: %+v", config)
	}
	if config.ViewMode != ViewBoard || databaseViewType(config.ViewMode) != ViewTable {
		t.Fatalf("view mode compatibility encoding missing: %+v", config)
	}
	if config.Groups[1].Display != "expanded" ||
		config.Groups[1].SortDirection != "asc" {
		t.Fatalf("group defaults were not normalized: %+v", config.Groups[1])
	}
	if !json.Valid(canonical) {
		t.Fatalf("canonical config is not JSON: %s", canonical)
	}
}

func TestParseViewConfigRejectsInvalidSpecializedViews(t *testing.T) {
	fields := testFields()
	cases := []struct {
		name     string
		viewType string
		raw      string
	}{
		{
			name: "calendar without date field", viewType: ViewCalendar,
			raw: `{"columns":["title"]}`,
		},
		{
			name: "calendar with non-date field", viewType: ViewCalendar,
			raw: `{"columns":["title"],"date_field":"title"}`,
		},
		{
			name: "board without group", viewType: ViewBoard,
			raw: `{"columns":["title"]}`,
		},
		{
			name: "form omits required field", viewType: ViewForm,
			raw: `{"columns":["status"],"form_fields":["status"],"public":true}`,
		},
		{
			name: "dashboard duplicate view", viewType: ViewDashboard,
			raw: `{"view_ids":["0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
				"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01"]}`,
		},
		{
			name: "native type with compatibility mode", viewType: ViewTable,
			raw: `{"columns":["title"],"view_mode":"board"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := parseViewConfig(
				json.RawMessage(tc.raw), fields, tc.viewType,
			); !errors.Is(err, ErrInvalidView) {
				t.Fatalf("expected ErrInvalidView, got %v", err)
			}
		})
	}
}

func TestParseViewConfigFormAndDashboard(t *testing.T) {
	form, _, err := parseViewConfig(
		json.RawMessage(`{
			"columns":["title","due"],"form_fields":["title","due"],"public":true
		}`),
		testFields(), ViewForm,
	)
	if err != nil {
		t.Fatal(err)
	}
	if form.SubmitLabel != "提交" || form.SuccessMessage != "提交成功" {
		t.Fatalf("form defaults not applied: %+v", form)
	}

	first, second := uuid.New(), uuid.New()
	raw, err := json.Marshal(ViewConfig{
		ViewIDs: []uuid.UUID{first, second}, GlobalFilterFields: []string{"status"},
		LayoutColumns: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	dashboard, _, err := parseViewConfig(raw, testFields(), ViewDashboard)
	if err != nil {
		t.Fatal(err)
	}
	if len(dashboard.ViewIDs) != 2 || dashboard.LayoutColumns != 3 {
		t.Fatalf("unexpected dashboard config: %+v", dashboard)
	}
}

func TestRecordWhereBuildsNestedParameterizedFilter(t *testing.T) {
	filter := &ViewFilter{
		Logic: "and",
		Conditions: []ViewFilter{
			{Field: "status", Operator: "contains", Value: json.RawMessage(`"open"`)},
			{
				Logic: "or",
				Conditions: []ViewFilter{
					{Field: "score", Operator: "gte", Value: json.RawMessage(`10`)},
					{Field: "due", Operator: "before", Value: json.RawMessage(`"2027-01-01"`)},
				},
			},
		},
	}
	where, args, err := recordWhere(uuid.New(), filter)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"ILIKE", " AND ", " OR ", "::numeric", "values_json->>"} {
		if !strings.Contains(where, fragment) {
			t.Fatalf("where clause missing %q: %s", fragment, where)
		}
	}
	if len(args) != 7 {
		t.Fatalf("expected dataset + 3 field/value pairs, got %d: %#v", len(args), args)
	}
	if strings.Contains(where, "status") || strings.Contains(where, "score") {
		t.Fatalf("field names were interpolated instead of parameterized: %s", where)
	}
}
