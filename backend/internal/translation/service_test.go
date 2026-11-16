package translation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/anby/wiki/backend/internal/ai"
	"reflect"
	"strings"
	"testing"
)

type generatorFunc func(context.Context, ai.Request) (*ai.Result, error)

func (f generatorFunc) Generate(ctx context.Context, r ai.Request) (*ai.Result, error) {
	return f(ctx, r)
}

const sampleAST = `{"type":"document","schema_version":1,"children":[{"id":"00000000-0000-7000-8000-000000000001","type":"heading","level":2,"content":[{"type":"text","text":"Hello","marks":["bold"]}]},{"id":"00000000-0000-7000-8000-000000000002","type":"paragraph","content":[{"type":"external_link","url":"https://example.com/a","display_text":"Example"},{"type":"inline_code","text":"const x = 1"},{"type":"text","text":"keep()","marks":["code"]}]},{"id":"00000000-0000-7000-8000-000000000003","type":"code","language":"go","content":"fmt.Println(1)"}]}`

func TestTranslatePreservesStructure(t *testing.T) {
	for _, language := range []string{"en", "zh-Hans"} {
		t.Run(language, func(t *testing.T) {
			calls := 0
			g := generatorFunc(func(_ context.Context, r ai.Request) (*ai.Result, error) {
				calls++
				if r.Variables["language"] != language {
					t.Fatal("wrong language")
				}
				var texts []string
				if err := json.Unmarshal([]byte(r.Variables["texts"].(string)), &texts); err != nil {
					t.Fatal(err)
				}
				for i, text := range texts {
					if strings.Contains(text, "const x") || strings.Contains(text, "fmt.Println") || text == "keep()" {
						t.Fatal("code sent for translation")
					}
					texts[i] = "译" + text
				}
				raw, _ := json.Marshal(map[string]any{"texts": texts})
				return &ai.Result{JSON: raw}, nil
			})
			title, out, err := Translate(context.Background(), g, "Title", json.RawMessage(sampleAST), language)
			if err != nil {
				t.Fatal(err)
			}
			if title != "译Title" || calls != 1 {
				t.Fatalf("title=%s calls=%d", title, calls)
			}
			expected := strings.ReplaceAll(strings.ReplaceAll(sampleAST, `"Hello"`, `"译Hello"`), `"Example"`, `"译Example"`)
			var a, b any
			_ = json.Unmarshal(out, &a)
			_ = json.Unmarshal([]byte(expected), &b)
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("structure changed: %s", out)
			}
		})
	}
}
func TestTranslateRejectsIncompleteOutput(t *testing.T) {
	for _, response := range []string{`{"texts":[]}`, `{"texts":["","","",""]}`, `{"texts":["only title"]}`} {
		g := generatorFunc(func(context.Context, ai.Request) (*ai.Result, error) {
			return &ai.Result{JSON: json.RawMessage(response)}, nil
		})
		if _, _, err := Translate(context.Background(), g, "Title", json.RawMessage(sampleAST), "en"); !errors.Is(err, ai.ErrInvalidOutput) {
			t.Fatalf("err=%v", err)
		}
	}
}

func TestHTMLTranslationKeepsTagsAndCode(t *testing.T) {
	object := map[string]any{"html": `<div class="a"><b>Hello</b><code>keep()</code><script>run()</script><a href="/x">World</a></div>`}
	var slots []slot
	var restore []func()
	htmlSlots(object, &slots, &restore)
	if len(slots) != 2 {
		t.Fatalf("slots=%d", len(slots))
	}
	for _, s := range slots {
		s.object[s.key] = "译" + s.object[s.key].(string)
	}
	for _, apply := range restore {
		apply()
	}
	expected := `<div class="a"><b>译Hello</b><code>keep()</code><script>run()</script><a href="/x">译World</a></div>`
	if object["html"] != expected {
		t.Fatalf("%s", object["html"])
	}
}

func TestTranslateEquationAndButtonText(t *testing.T) {
	raw := json.RawMessage(`{"type":"document","schema_version":1,"children":[
 {"type":"equation","id":"00000000-0000-7000-8000-000000000001","expression":"E=mc^2","caption":"Energy"},
 {"type":"button","id":"00000000-0000-7000-8000-000000000002","label":"Start","actions":[
 {"type":"notification","message":"Done","tone":"success"},
 {"type":"insert_blocks","target_page_id":"00000000-0000-7000-8000-000000000003","blocks":[{"type":"paragraph","id":"00000000-0000-7000-8000-000000000004","content":[{"type":"text","text":"Inserted"}]}]},
 {"type":"update_dataset_record","record_id":"00000000-0000-7000-8000-000000000005","values":{"message":"Stored value"}}
 ]}]}`)
	g := generatorFunc(func(_ context.Context, r ai.Request) (*ai.Result, error) {
		var texts []string
		if err := json.Unmarshal([]byte(r.Variables["texts"].(string)), &texts); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(texts, []string{"Title", "Energy", "Start", "Done", "Inserted"}) {
			t.Fatalf("unexpected slots: %v", texts)
		}
		for i := range texts {
			texts[i] = "译" + texts[i]
		}
		out, _ := json.Marshal(map[string]any{"texts": texts})
		return &ai.Result{JSON: out}, nil
	})
	_, out, err := Translate(context.Background(), g, "Title", raw, "zh-Hans")
	if err != nil {
		t.Fatal(err)
	}
	expected := string(raw)
	for _, text := range []string{"Energy", "Start", "Done", "Inserted"} {
		expected = strings.ReplaceAll(expected, `"`+text+`"`, `"译`+text+`"`)
	}
	var actual, want any
	_ = json.Unmarshal(out, &actual)
	_ = json.Unmarshal([]byte(expected), &want)
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("nontext data changed: %s", out)
	}
}
