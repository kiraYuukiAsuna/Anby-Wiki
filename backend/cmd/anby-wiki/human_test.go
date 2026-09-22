package main

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestParseHumanAuthExchange(t *testing.T) {
	input, mode, help, err := parseHumanArguments([]string{
		"auth", "exchange", "--base-url", "https://anbywiki.example.com",
		"--code", "anby_code_test", "--no-persist", "--json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if help != "" {
		t.Fatalf("unexpected help: %s", help)
	}
	if mode != outputJSON {
		t.Fatalf("mode=%v want outputJSON", mode)
	}
	if input.Action != "auth.exchange" || input.Code != "anby_code_test" ||
		input.BaseURL != "https://anbywiki.example.com" {
		t.Fatalf("unexpected input: %#v", input)
	}
	if input.Persist == nil || *input.Persist {
		t.Fatalf("persist=%v want false", input.Persist)
	}
}

func TestParseHumanCollaborationRun(t *testing.T) {
	pageID := uuid.NewString()
	clientID := uuid.NewString()
	updateID := uuid.NewString()
	input, mode, help, err := parseHumanArguments([]string{
		"collaboration", "run",
		"--page-id", pageID,
		"--client-id", clientID,
		"--last-sequence", "12",
		"--message", `{"type":"update","update_id":"` + updateID + `","data_base64":"YQ=="}`,
		"--json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if help != "" {
		t.Fatalf("unexpected help: %s", help)
	}
	if mode != outputJSON {
		t.Fatalf("mode=%v want outputJSON", mode)
	}
	if input.Action != "collaboration.run" || input.PageID != pageID ||
		input.ClientID != clientID || input.LastSequence != 12 {
		t.Fatalf("unexpected input: %#v", input)
	}
	if len(input.Messages) != 1 || input.Messages[0].UpdateID != updateID ||
		input.Messages[0].DataBase64 != "YQ==" {
		t.Fatalf("messages=%#v", input.Messages)
	}
}

func TestParseHumanDirectOperationParameters(t *testing.T) {
	pageID := uuid.NewString()
	input, mode, help, err := parseHumanArguments([]string{
		"get-page-by-id", "--id", pageID, "--content-mode", "html", "--json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if help != "" {
		t.Fatalf("unexpected help: %s", help)
	}
	if mode != outputJSON {
		t.Fatalf("mode=%v want outputJSON", mode)
	}
	if input.Action != "operation.call" || input.OperationID != "getPageByID" {
		t.Fatalf("unexpected input: %#v", input)
	}
	if input.Path["id"] != pageID {
		t.Fatalf("path=%#v", input.Path)
	}
	if input.Query["content_mode"] != "html" {
		t.Fatalf("query=%#v", input.Query)
	}
}

func TestParseHumanOperationBodyFields(t *testing.T) {
	input, mode, help, err := parseHumanArguments([]string{
		"create-page",
		"--namespace", "main",
		"--title", "CLI Page",
		"--language", "zh-Hans",
		"--content-model", "block-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if help != "" {
		t.Fatalf("unexpected help: %s", help)
	}
	if mode != outputHuman {
		t.Fatalf("mode=%v want outputHuman", mode)
	}
	if input.Action != "operation.call" || input.OperationID != "createPage" {
		t.Fatalf("unexpected input: %#v", input)
	}
	body := map[string]any{}
	if err := json.Unmarshal(input.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["namespace"] != "main" || body["title"] != "CLI Page" ||
		body["language"] != "zh-Hans" || body["content_model"] != "block-v1" {
		t.Fatalf("body=%#v", body)
	}
}

func TestParseHumanExplicitCallWithMultipartFile(t *testing.T) {
	input, _, help, err := parseHumanArguments([]string{
		"call", "create-import-upload-job",
		"--title", "Import source",
		"--route-mode", "auto",
		"--file", "file=./source.pdf",
		"--timeout", "600",
	})
	if err != nil {
		t.Fatal(err)
	}
	if help != "" {
		t.Fatalf("unexpected help: %s", help)
	}
	if input.OperationID != "createImportUploadJob" || input.TimeoutSeconds != 600 {
		t.Fatalf("unexpected input: %#v", input)
	}
	if input.Files["file"] != "./source.pdf" {
		t.Fatalf("files=%#v", input.Files)
	}
	body := map[string]any{}
	if err := json.Unmarshal(input.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["title"] != "Import source" || body["route_mode"] != "auto" {
		t.Fatalf("body=%#v", body)
	}
}
