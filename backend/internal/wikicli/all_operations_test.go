package wikicli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/clicontract"
	"github.com/google/uuid"
)

type probeRequest struct {
	method      string
	path        string
	query       string
	contentType string
	authorized  bool
}

func TestAllOperationsReachHTTPTransport(t *testing.T) {
	requests := make(chan probeRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		requests <- probeRequest{
			method:      request.Method,
			path:        request.URL.EscapedPath(),
			query:       request.URL.RawQuery,
			contentType: request.Header.Get("Content-Type"),
			authorized: request.Header.Get("Authorization") ==
				"Bearer anby_token_transport_probe",
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-ID", "transport-probe")
		writer.WriteHeader(http.StatusTeapot)
		_, _ = writer.Write([]byte(
			`{"code":"transport_probe","message":"transport reached"}`,
		))
	}))
	defer server.Close()

	app := testApp(t)
	configPath := filepath.Join(t.TempDir(), "cli.json")
	if err := saveConfig(configPath, Config{
		BaseURL: server.URL, Token: "anby_token_transport_probe",
	}); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(t.TempDir(), "probe.txt")
	if err := os.WriteFile(filePath, []byte("transport probe"), 0o600); err != nil {
		t.Fatal(err)
	}

	operations := app.contract.List()
	if len(operations) != 155 {
		t.Fatalf("operation count=%d want=155", len(operations))
	}
	for _, operation := range operations {
		t.Run(operation.ID, func(t *testing.T) {
			input := operationProbeInput(t, operation, configPath, filePath)
			result, exitCode := app.Execute(context.Background(), input)
			if exitCode != 1 || result.OK || result.Error == nil ||
				result.Error.Code != "transport_probe" {
				t.Fatalf("error=%#v result=%#v exit=%d",
					result.Error, result, exitCode)
			}
			if result.Meta == nil ||
				result.Meta.HTTPStatus != http.StatusTeapot ||
				result.Meta.RequestID != "transport-probe" {
				t.Fatalf("metadata=%#v", result.Meta)
			}
			request := <-requests
			if request.method != operation.Method {
				t.Fatalf("method=%s want=%s", request.method, operation.Method)
			}
			if strings.Contains(request.path, "{") {
				t.Fatalf("unresolved path placeholder: %s", request.path)
			}
			if !request.authorized {
				t.Fatal("request did not use the configured Bearer token")
			}
			if operation.RequestBody != nil &&
				containsFold(operation.RequestBody.ContentTypes, "multipart/form-data") &&
				!strings.HasPrefix(request.contentType, "multipart/form-data;") {
				t.Fatalf("content-type=%q want multipart/form-data", request.contentType)
			}
		})
	}
}

func TestAllOperationsAgainstAPI(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("CLI_E2E_BASE_URL"), "/")
	token := strings.TrimSpace(os.Getenv("CLI_E2E_TOKEN"))
	if baseURL == "" || token == "" {
		t.Skip("set CLI_E2E_BASE_URL and CLI_E2E_TOKEN")
	}

	app := testApp(t)
	configPath := filepath.Join(t.TempDir(), "cli.json")
	if err := saveConfig(configPath, Config{
		BaseURL: baseURL, Token: token,
	}); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(t.TempDir(), "probe.txt")
	if err := os.WriteFile(filePath, []byte("CLI E2E transport probe"), 0o600); err != nil {
		t.Fatal(err)
	}

	operations := app.contract.List()
	for index, operation := range operations {
		if operation.ID == "revokeCurrentCLIToken" {
			operations = append(
				append(operations[:index], operations[index+1:]...),
				operation,
			)
			break
		}
	}
	statuses := map[int]int{}
	for _, operation := range operations {
		t.Run(operation.ID, func(t *testing.T) {
			input := operationProbeInput(t, operation, configPath, filePath)
			result, exitCode := app.Execute(context.Background(), input)
			if exitCode == 2 || result.Meta == nil {
				t.Fatalf("operation did not reach a valid API response: error=%#v exit=%d",
					result.Error, exitCode)
			}
			statuses[result.Meta.HTTPStatus]++
			if result.Meta.HTTPStatus >= 500 &&
				!(operation.ID == "testAIConfig" &&
					(result.Meta.HTTPStatus == http.StatusBadGateway ||
						result.Meta.HTTPStatus == http.StatusGatewayTimeout)) {
				t.Fatalf("unexpected server failure: status=%d error=%#v",
					result.Meta.HTTPStatus, result.Error)
			}
		})
	}
	t.Logf("exercised %d CLI operations against API; statuses=%v",
		len(operations), statuses)
}

func operationProbeInput(
	t *testing.T,
	operation clicontract.Descriptor,
	configPath, filePath string,
) Input {
	t.Helper()
	input := Input{
		Action: "operation.call", OperationID: operation.ID,
		ConfigPath: configPath,
	}
	for _, parameter := range operation.Parameters {
		if !parameter.Required {
			continue
		}
		value := probeSchemaValue(t, parameter.Schema)
		switch parameter.In {
		case "path":
			if input.Path == nil {
				input.Path = map[string]any{}
			}
			input.Path[parameter.Name] = value
		case "query":
			if input.Query == nil {
				input.Query = map[string]any{}
			}
			input.Query[parameter.Name] = value
		case "header":
			if input.Headers == nil {
				input.Headers = map[string]any{}
			}
			input.Headers[parameter.Name] = value
		}
	}
	if operation.RequestBody == nil || !operation.RequestBody.Required {
		return input
	}

	contentType := operation.RequestBody.ContentTypes[0]
	for _, candidate := range operation.RequestBody.ContentTypes {
		if candidate == "application/json" {
			contentType = candidate
			break
		}
	}
	schema := operation.RequestBody.Schemas[contentType]
	body := probeSchemaValue(t, schema)
	if contentType == "multipart/form-data" {
		object, ok := body.(map[string]any)
		if !ok {
			t.Fatalf("multipart body is %T", body)
		}
		input.Files = map[string]string{}
		for name, property := range schemaProperties(schema) {
			if schemaFormat(property) != "binary" {
				continue
			}
			input.Files[name] = filePath
			delete(object, name)
		}
		if len(input.Files) == 0 {
			t.Fatal("multipart request has no binary file property")
		}
		body = object
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	input.Body = raw
	return input
}

func probeSchemaValue(t *testing.T, raw any) any {
	t.Helper()
	schema, ok := raw.(map[string]any)
	if !ok {
		return "probe"
	}
	if value, exists := schema["const"]; exists {
		return value
	}
	if values, ok := schema["enum"].([]any); ok && len(values) > 0 {
		return values[0]
	}
	if _, ok := schema["allOf"].([]any); ok {
		return probeSchemaValue(t, mergeProbeAllOf(schema))
	}
	for _, keyword := range []string{"oneOf", "anyOf"} {
		if branches, ok := schema[keyword].([]any); ok && len(branches) > 0 {
			return probeSchemaValue(t, branches[0])
		}
	}

	schemaType := probeSchemaType(schema["type"])
	if schemaType == "" {
		if _, exists := schema["properties"]; exists {
			schemaType = "object"
		}
	}
	switch schemaType {
	case "object":
		object := map[string]any{}
		properties := schemaProperties(schema)
		for _, name := range schemaRequired(schema) {
			if _, exists := object[name]; exists {
				continue
			}
			object[name] = probeSchemaValue(t, properties[name])
		}
		minimum := schemaInt(schema["minProperties"])
		for name, property := range properties {
			if len(object) >= minimum {
				break
			}
			if _, exists := object[name]; !exists {
				object[name] = probeSchemaValue(t, property)
			}
		}
		if len(object) < minimum {
			var value any = "probe"
			if additional, ok := schema["additionalProperties"].(map[string]any); ok {
				value = probeSchemaValue(t, additional)
			}
			object["probe"] = value
		}
		return object
	case "array":
		count := schemaInt(schema["minItems"])
		result := make([]any, 0, count)
		for index := 0; index < count; index++ {
			result = append(result, probeSchemaValue(t, schema["items"]))
		}
		return result
	case "integer":
		value := schemaInt(schema["minimum"])
		if value == 0 {
			value = 1
		}
		return json.Number(strconv.Itoa(value))
	case "number":
		if minimum, ok := probeSchemaNumber(schema["minimum"]); ok {
			return minimum
		}
		return 1.0
	case "boolean":
		return true
	case "null":
		return nil
	default:
		return probeString(schema)
	}
}

func mergeProbeAllOf(schema map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range schema {
		if key != "allOf" {
			result[key] = value
		}
	}
	branches, _ := schema["allOf"].([]any)
	for _, rawBranch := range branches {
		branch, _ := rawBranch.(map[string]any)
		if _, nested := branch["allOf"].([]any); nested {
			branch = mergeProbeAllOf(branch)
		}
		mergeProbeSchema(result, branch)
	}
	return result
}

func mergeProbeSchema(target, source map[string]any) {
	for key, value := range source {
		switch key {
		case "properties":
			targetProperties, _ := target[key].(map[string]any)
			if targetProperties == nil {
				targetProperties = map[string]any{}
			}
			sourceProperties, _ := value.(map[string]any)
			for name, property := range sourceProperties {
				targetProperties[name] = property
			}
			target[key] = targetProperties
		case "required":
			current, _ := target[key].([]any)
			seen := map[string]bool{}
			for _, item := range current {
				if text, ok := item.(string); ok {
					seen[text] = true
				}
			}
			for _, item := range value.([]any) {
				text, _ := item.(string)
				if !seen[text] {
					current = append(current, item)
					seen[text] = true
				}
			}
			target[key] = current
		default:
			target[key] = value
		}
	}
}

func probeString(schema map[string]any) string {
	switch schemaFormat(schema) {
	case "uuid":
		return uuid.MustParse("0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01").String()
	case "date-time":
		return "2026-08-19T00:00:00Z"
	case "date":
		return "2026-08-19"
	case "email":
		return "probe@example.invalid"
	case "uri", "url":
		return "https://example.invalid/probe"
	case "byte":
		return "cHJvYmU="
	case "binary":
		return "/tmp/probe"
	case "password":
		return "probe-password-123"
	}
	pattern, _ := schema["pattern"].(string)
	switch {
	case strings.Contains(pattern, "anby_code_"):
		return "anby_code_probe"
	case strings.Contains(pattern, "anby_token_"):
		return "anby_token_probe"
	case strings.Contains(pattern, "0-9a-f") &&
		schemaInt(schema["minLength"]) == 64:
		return strings.Repeat("0", 64)
	case strings.HasPrefix(pattern, "^[A-Za-z0-9]"):
		return "probe_user"
	case strings.HasPrefix(pattern, "^[a-z]"):
		return "probe"
	}
	length := schemaInt(schema["minLength"])
	if length < 1 {
		length = 1
	}
	return strings.Repeat("x", length)
}

func probeSchemaType(raw any) string {
	if value, ok := raw.(string); ok {
		return value
	}
	if values, ok := raw.([]any); ok {
		for _, value := range values {
			if text, ok := value.(string); ok && text != "null" {
				return text
			}
		}
	}
	return ""
}

func probeSchemaNumber(raw any) (float64, bool) {
	switch value := raw.(type) {
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case float64:
		return value, true
	case json.Number:
		number, err := value.Float64()
		return number, err == nil
	default:
		return 0, false
	}
}

func schemaInt(raw any) int {
	value, _ := probeSchemaNumber(raw)
	return int(value)
}

func schemaProperties(raw any) map[string]any {
	schema, _ := raw.(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	return properties
}

func schemaRequired(raw any) []string {
	schema, _ := raw.(map[string]any)
	values, _ := schema["required"].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func schemaFormat(raw any) string {
	schema, _ := raw.(map[string]any)
	value, _ := schema["format"].(string)
	return value
}
