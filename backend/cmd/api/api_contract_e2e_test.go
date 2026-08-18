package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type apiE2EOperation struct {
	ID     string
	Method string
	Path   string
}

func TestAPIContractE2E(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("API_E2E_BASE_URL"), "/")
	password := os.Getenv("API_E2E_PASSWORD")
	if baseURL == "" || password == "" {
		t.Skip("set API_E2E_BASE_URL and API_E2E_PASSWORD")
	}

	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	admin, adminActor := apiE2EAdmin(t, baseURL, password)
	editor, _ := registerE2EUser(
		t, baseURL, "api_editor_"+runID,
		"api_editor_"+runID+"@example.invalid", password,
	)

	covered := map[string]bool{
		"register": true,
	}
	assertE2EStatus(t, doE2ERequest(t, admin, http.MethodGet,
		baseURL+"/api/v1/auth/session", nil, nil), http.StatusOK)
	covered["getSession"] = true

	assertE2EStatus(t, doE2ERequest(t, editor, http.MethodPost,
		baseURL+"/api/v1/auth/logout", nil, nil), http.StatusNoContent)
	covered["logout"] = true
	assertE2EStatus(t, doE2ERequest(t, editor, http.MethodPost,
		baseURL+"/api/v1/auth/login", map[string]any{
			"identifier": "api_editor_" + runID,
			"password":   password,
		}, nil), http.StatusOK)
	covered["login"] = true

	operations := loadOpenAPIOperations(t)
	for _, operation := range operations {
		if covered[operation.ID] {
			continue
		}
		endpoint := baseURL + e2eOperationPath(operation)
		var body any
		if operation.Method != http.MethodGet &&
			operation.Method != http.MethodDelete {
			body = map[string]any{}
		}
		headers := http.Header{}
		if operation.ID == "createProposal" {
			headers.Set("Idempotency-Key", uuid.NewString())
		}
		response := doE2ERequest(
			t, admin, operation.Method, endpoint, body, headers,
		)
		responseBody := readE2EBody(response.Body)
		response.Body.Close()
		if response.StatusCode >= 500 &&
			!expectedE2EDependencyFailure(operation.ID, response.StatusCode) {
			t.Errorf("%s %s (%s) returned %d: %s",
				operation.Method, operation.Path, operation.ID,
				response.StatusCode, responseBody)
		}
		covered[operation.ID] = true
	}

	var missing []string
	for _, operation := range operations {
		if !covered[operation.ID] {
			missing = append(missing, operation.ID)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("operations not exercised: %s", strings.Join(missing, ", "))
	}
	t.Logf("exercised %d OpenAPI operations; admin actor=%s",
		len(operations), adminActor)
}

func expectedE2EDependencyFailure(operationID string, status int) bool {
	return operationID == "testAIConfig" &&
		(status == http.StatusBadGateway || status == http.StatusGatewayTimeout)
}

func apiE2EAdmin(
	t *testing.T,
	baseURL, password string,
) (*http.Client, uuid.UUID) {
	t.Helper()
	runID := strings.ToLower(os.Getenv("API_E2E_RUN_ID"))
	runID = regexp.MustCompile(`[^a-z0-9]`).ReplaceAllString(runID, "")
	if runID == "" {
		runID = "default"
	}
	if len(runID) > 12 {
		runID = runID[:12]
	}
	username := "api_e2e_admin_" + runID
	email := username + "@example.invalid"
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: collaborationE2ETimeout}
	response := postE2ERaw(t, client, baseURL+"/api/v1/auth/register",
		map[string]any{
			"username": username, "email": email, "password": password,
			"display_name": "API E2E Admin",
		})
	defer response.Body.Close()
	if response.StatusCode == http.StatusCreated {
		var result e2eAuthResult
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return client, result.ActorID
	}
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("register shared admin status=%d: %s",
			response.StatusCode, readE2EBody(response.Body))
	}
	response.Body.Close()
	login := postE2ERaw(t, client, baseURL+"/api/v1/auth/login",
		map[string]any{"identifier": username, "password": password})
	defer login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login shared admin status=%d: %s",
			login.StatusCode, readE2EBody(login.Body))
	}
	var result e2eAuthResult
	if err := json.NewDecoder(login.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return client, result.ActorID
}

func loadOpenAPIOperations(t *testing.T) []apiE2EOperation {
	t.Helper()
	openAPIPath := filepath.Join("..", "..", "..", "contracts", "openapi", "openapi.yaml")
	file, err := os.Open(openAPIPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	pathPattern := regexp.MustCompile(`^  (/[^:]+):\s*$`)
	methodPattern := regexp.MustCompile(`^    (get|post|put|patch|delete):\s*$`)
	operationPattern := regexp.MustCompile(`^      operationId:\s*(\S+)\s*$`)
	var currentPath, currentMethod string
	var operations []apiE2EOperation
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if match := pathPattern.FindStringSubmatch(line); match != nil {
			currentPath = match[1]
			continue
		}
		if match := methodPattern.FindStringSubmatch(line); match != nil {
			currentMethod = strings.ToUpper(match[1])
			continue
		}
		if match := operationPattern.FindStringSubmatch(line); match != nil {
			operations = append(operations, apiE2EOperation{
				ID: match[1], Method: currentMethod, Path: currentPath,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(operations) == 0 {
		t.Fatal("no OpenAPI operations found")
	}
	return operations
}

func e2eOperationPath(operation apiE2EOperation) string {
	zero := uuid.Nil.String()
	replacements := map[string]string{
		"{id}":          zero,
		"{rid}":         zero,
		"{revision_id}": zero,
		"{conflict_id}": zero,
		"{proposal_id}": zero,
		"{actor_id}":    zero,
		"{entity_id}":   zero,
		"{page_id}":     zero,
		"{alias_id}":    zero,
		"{block_id}":    zero,
		"{version}":     "1",
		"{section_key}": "missing",
		"{slug}":        "missing",
	}
	result := operation.Path
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	query := url.Values{}
	query.Set("page_size", "1")
	switch operation.ID {
	case "getPageByTitle":
		query.Set("namespace", "main")
		query.Set("title", "missing-api-e2e-page")
	case "diffRevisions":
		query.Set("from", zero)
		query.Set("to", zero)
	case "removeEntityLabel":
		query.Set("language", "und")
		query.Set("label", "missing")
	}
	return result + "?" + query.Encode()
}

func doE2ERequest(
	t *testing.T,
	client *http.Client,
	method, endpoint string,
	body any,
	headers http.Header,
) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, endpoint, err)
	}
	return response
}

func assertE2EStatus(t *testing.T, response *http.Response, want int) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("status=%d want=%d: %s",
			response.StatusCode, want, readE2EBody(response.Body))
	}
}
