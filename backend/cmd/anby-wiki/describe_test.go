package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/clicontract"
)

func TestHumanDescribeCreatePage(t *testing.T) {
	contract, err := clicontract.Load()
	if err != nil {
		t.Fatal(err)
	}
	operation, err := contract.Describe("createPage")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := writeOperationDescription(&output, operation); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"$.namespace", "$.title", "$.language", "$.content_model", "minLength=1", "maxLength=255", "example=main", "命名空间", "--field name=value", "--body @file.json"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q in %s", want, output.String())
		}
	}
	for _, args := range [][]string{{"create-page", "--help"}, {"call", "create-page", "--help"}} {
		input, _, help, err := parseHumanArguments(args)
		if err != nil || help != output.String() || input.Action != "" {
			t.Fatalf("args=%v input=%+v help=%q err=%v", args, input, help, err)
		}
	}
}

func TestHumanDescribeNestedFieldsAndParameters(t *testing.T) {
	var output bytes.Buffer
	descriptor := map[string]any{}
	// Decode through the same JSON boundary as real descriptors.
	if !decodeData(map[string]any{
		"operation_id": "example", "parameters": []any{map[string]any{"name": "mode", "in": "query", "required": true, "schema": map[string]any{"type": "string", "enum": []string{"html", "ast"}, "default": "html"}}},
		"request_body": map[string]any{"content_types": []string{"application/json"}, "schemas": map[string]any{"application/json": map[string]any{
			"type": "object", "required": []string{"items"}, "properties": map[string]any{
				"items":  map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"name"}, "properties": map[string]any{"name": map[string]any{"type": "string"}}}},
				"choice": map[string]any{"oneOf": []any{map[string]any{"const": "a"}, map[string]any{"$ref": "#/components/schemas/Recursive"}}},
				"file":   map[string]any{"type": "string", "format": "binary"},
			},
		}}},
	}, &descriptor) {
		t.Fatal("decode failed")
	}
	if err := writeOperationDescription(&output, descriptor); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--query mode=VALUE", `enum=["html","ast"]`, "default=html", "$.items[]", "$.items[].name", "$.choice (oneOf 1)", "$ref=#/components/schemas/Recursive", "format=binary", "--file field="} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q in %s", want, output.String())
		}
	}
	for _, line := range strings.Split(output.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 2 && (fields[0] == "$.items" || fields[0] == "$.items[].name") && fields[2] != "true" {
			t.Errorf("required field: %s", line)
		}
	}
}

func TestHumanDescribeAllOperations(t *testing.T) {
	contract, err := clicontract.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range contract.List() {
		t.Run(operation.ID, func(t *testing.T) {
			descriptor, err := contract.Describe(operation.ID)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err := writeOperationDescription(&output, descriptor); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "Usage: anby-wiki "+commandName(operation.ID)) {
				t.Fatal("missing usage")
			}
		})
	}
}
