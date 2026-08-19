package clicontract

import "testing"

func TestLoadAndValidateOperations(t *testing.T) {
	contract, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(contract.List()); got != 149 {
		t.Fatalf("operation count=%d, want 149", got)
	}
	page, err := contract.Describe("createPage")
	if err != nil {
		t.Fatal(err)
	}
	if page.Method != "POST" || page.Path != "/api/v1/pages" {
		t.Fatalf("unexpected createPage descriptor: %#v", page)
	}
	if err := contract.ValidateCall("createPage", Call{
		BodyPresent: true,
		ContentType: "application/json",
		Body: map[string]any{
			"namespace": "main",
			"title":     "CLI contract",
		},
	}); err != nil {
		t.Fatalf("valid createPage request rejected: %v", err)
	}
	if err := contract.ValidateCall("createPage", Call{
		BodyPresent: true,
		ContentType: "application/json",
		Body: map[string]any{
			"namespace": "main",
			"title":     "",
			"unknown":   true,
		},
	}); err == nil {
		t.Fatal("invalid createPage request accepted")
	}
}

func TestCLIOnlyOperationsAreDescribed(t *testing.T) {
	contract, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{
		"exchangeCLIAuthCode",
		"revokeCurrentCLIToken",
	} {
		operation, err := contract.Describe(id)
		if err != nil {
			t.Fatal(err)
		}
		if !operation.CLIOnly {
			t.Fatalf("%s is not marked CLI-only", id)
		}
	}
}

func TestAllOperationSchemasCompile(t *testing.T) {
	contract, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range contract.order {
		if err := contract.operations[id].ensureCompiled(); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
}

func TestNullableEnumAcceptsNull(t *testing.T) {
	schema, err := compileJSONSchema(
		map[string]any{},
		normalizeSchema(map[string]any{
			"type": "string", "enum": []any{"active", "revoked"},
			"nullable": true,
		}),
		"nullable-enum",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateValue(schema, nil); err != nil {
		t.Fatalf("nullable enum rejected null: %v", err)
	}
	if err := validateValue(schema, "active"); err != nil {
		t.Fatalf("nullable enum rejected declared value: %v", err)
	}
	if err := validateValue(schema, "missing"); err == nil {
		t.Fatal("nullable enum accepted undeclared string")
	}
}
