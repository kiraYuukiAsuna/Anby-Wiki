// Package wikicli implements the JSON-only agent CLI.
package wikicli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anby/wiki/backend/internal/clicontract"
)

const (
	defaultTimeout = 60 * time.Second
	maxTimeout     = 10 * time.Minute
	maxResponse    = 64 << 20
)

type Input struct {
	Action         string                 `json:"action"`
	BaseURL        string                 `json:"base_url,omitempty"`
	ConfigPath     string                 `json:"config_path,omitempty"`
	Code           string                 `json:"code,omitempty"`
	Persist        *bool                  `json:"persist,omitempty"`
	OperationID    string                 `json:"operation_id,omitempty"`
	Tag            string                 `json:"tag,omitempty"`
	Search         string                 `json:"search,omitempty"`
	Path           map[string]any         `json:"path,omitempty"`
	Query          map[string]any         `json:"query,omitempty"`
	Headers        map[string]any         `json:"headers,omitempty"`
	Body           json.RawMessage        `json:"body,omitempty"`
	Files          map[string]string      `json:"files,omitempty"`
	TimeoutSeconds int                    `json:"timeout_seconds,omitempty"`
	PageID         string                 `json:"page_id,omitempty"`
	ClientID       string                 `json:"client_id,omitempty"`
	LastSequence   int64                  `json:"last_sequence,omitempty"`
	Messages       []CollaborationMessage `json:"messages,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type Meta struct {
	OperationID string              `json:"operation_id,omitempty"`
	HTTPStatus  int                 `json:"http_status,omitempty"`
	RequestID   string              `json:"request_id,omitempty"`
	ContentType string              `json:"content_type,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
}

type Result struct {
	OK     bool   `json:"ok"`
	Action string `json:"action"`
	Data   any    `json:"data,omitempty"`
	Error  *Error `json:"error,omitempty"`
	Meta   *Meta  `json:"meta,omitempty"`
}

type Config struct {
	BaseURL     string     `json:"base_url"`
	Token       string     `json:"token,omitempty"`
	TokenPrefix string     `json:"token_prefix,omitempty"`
	ActorID     string     `json:"actor_id,omitempty"`
	DisplayName string     `json:"display_name,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type App struct {
	contract *clicontract.Contract
	version  string
	client   *http.Client
	getenv   func(string) string
}

type operationSummary struct {
	OperationID string   `json:"operation_id"`
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Summary     string   `json:"summary"`
	Tags        []string `json:"tags"`
	CLIOnly     bool     `json:"cli_only"`
}

type remoteResponse struct {
	Status      int
	ContentType string
	RequestID   string
	Headers     map[string][]string
	Body        any
}

type cliInputError struct {
	err error
}

func (e *cliInputError) Error() string {
	return e.err.Error()
}

func (e *cliInputError) Unwrap() error {
	return e.err
}

func New(version string) (*App, error) {
	contract, err := clicontract.Load()
	if err != nil {
		return nil, err
	}
	return &App{
		contract: contract,
		version:  version,
		client:   &http.Client{Timeout: defaultTimeout},
		getenv:   os.Getenv,
	}, nil
}

func DecodeInput(reader io.Reader) (Input, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 4<<20))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var input Input
	if err := decoder.Decode(&input); err != nil {
		return Input{}, fmt.Errorf("decode input: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Input{}, errors.New("decode input: expected one JSON object")
	}
	input.Action = strings.TrimSpace(input.Action)
	if input.Action == "" {
		return Input{}, errors.New("action is required")
	}
	return input, nil
}

func EncodeResult(writer io.Writer, result Result) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}

func (a *App) Execute(ctx context.Context, input Input) (Result, int) {
	switch input.Action {
	case "help":
		return HelpResult(a.version), 0
	case "version":
		return success(input.Action, map[string]string{"version": a.version}), 0
	case "operations.list":
		return a.listOperations(input), 0
	case "operation.describe":
		return a.describeOperation(input)
	case "operation.call":
		return a.callOperation(ctx, input)
	case "auth.exchange":
		return a.exchange(ctx, input)
	case "auth.status":
		return a.authStatus(ctx, input)
	case "auth.logout":
		return a.logout(ctx, input)
	case "collaboration.run":
		return a.runCollaboration(ctx, input)
	case "config.show":
		return a.showConfig(input)
	default:
		return failure(
			input.Action, "unknown_action",
			"unsupported action; use help, version, operations.list, operation.describe, operation.call, collaboration.run, auth.exchange, auth.status, auth.logout, or config.show",
			nil,
		), 2
	}
}

func HelpResult(version string) Result {
	return success("help", map[string]any{
		"version":     version,
		"description": "JSON-only CLI for agents to operate Anby Wiki through the full OpenAPI contract plus collaboration WebSocket.",
		"usage": []string{
			"anby-wiki < request.json",
			"anby-wiki --input request.json",
			"anby-wiki --help",
		},
		"quick_start": []map[string]any{
			{
				"step":    1,
				"title":   "Create an authorization code",
				"command": "Open https://anbywiki.example.com/settings/cli in a browser, then create a one-time code.",
			},
			{
				"step":    2,
				"title":   "Exchange the code",
				"command": `printf '%s\n' '{"action":"auth.exchange","base_url":"https://anbywiki.example.com","code":"anby_code_..."}' | anby-wiki`,
			},
			{
				"step":    3,
				"title":   "Inspect available APIs",
				"command": `printf '%s\n' '{"action":"operations.list"}' | anby-wiki`,
			},
			{
				"step":    4,
				"title":   "Inspect one API schema",
				"command": `printf '%s\n' '{"action":"operation.describe","operation_id":"createPage"}' | anby-wiki`,
			},
			{
				"step":    5,
				"title":   "Call an API",
				"command": `printf '%s\n' '{"action":"operation.call","operation_id":"getSession"}' | anby-wiki`,
			},
		},
		"input_envelope": map[string]any{
			"required": []string{"action"},
			"fields": []map[string]string{
				{"name": "action", "type": "string", "description": "CLI action name."},
				{"name": "base_url", "type": "string", "description": "Server base URL; overrides config and ANBY_WIKI_BASE_URL."},
				{"name": "config_path", "type": "string", "description": "Local config path; overrides ANBY_WIKI_CONFIG."},
				{"name": "code", "type": "string", "description": "One-time code for auth.exchange."},
				{"name": "persist", "type": "boolean", "description": "Whether auth.exchange writes the token to config; defaults to true."},
				{"name": "operation_id", "type": "string", "description": "OpenAPI operationId for operation.describe or operation.call."},
				{"name": "tag", "type": "string", "description": "Filter operations.list by OpenAPI tag."},
				{"name": "search", "type": "string", "description": "Filter operations.list by operation id, path, summary, method, or tag."},
				{"name": "path", "type": "object", "description": "Path parameters for operation.call."},
				{"name": "query", "type": "object", "description": "Query parameters for operation.call."},
				{"name": "headers", "type": "object", "description": "Request headers for operation.call."},
				{"name": "body", "type": "object", "description": "JSON body or multipart fields for operation.call."},
				{"name": "files", "type": "object", "description": "Multipart file fields; values are local file paths."},
				{"name": "timeout_seconds", "type": "integer", "description": "Request timeout; max 600."},
				{"name": "page_id", "type": "string", "description": "Page UUID for collaboration.run."},
				{"name": "client_id", "type": "string", "description": "Client UUID for collaboration.run."},
				{"name": "last_sequence", "type": "integer", "description": "Collaboration recovery sequence, starting at 0."},
				{"name": "messages", "type": "array", "description": "Collaboration messages to send after recovery."},
			},
		},
		"output_envelope": map[string]any{
			"success": map[string]any{
				"ok":     true,
				"action": "operation.call",
				"data":   map[string]string{"example": "result payload"},
				"meta": map[string]any{
					"operation_id": "getSession",
					"http_status":  200,
					"request_id":   "request id when provided by server",
					"content_type": "application/json",
				},
			},
			"failure": map[string]any{
				"ok":     false,
				"action": "operation.call",
				"error": map[string]string{
					"code":    "validation_failed",
					"message": "request validation failed: ...",
				},
			},
		},
		"actions": []map[string]any{
			{
				"name":        "help",
				"description": "Show this help envelope.",
				"example":     map[string]any{"action": "help"},
			},
			{
				"name":        "version",
				"description": "Show CLI version.",
				"example":     map[string]any{"action": "version"},
			},
			{
				"name":        "auth.exchange",
				"description": "Exchange a one-time authorization code from /settings/cli for a stored CLI token.",
				"example": map[string]any{
					"action":   "auth.exchange",
					"base_url": "https://anbywiki.example.com",
					"code":     "anby_code_...",
				},
			},
			{
				"name":        "auth.status",
				"description": "Call getSession with the configured CLI token.",
				"example":     map[string]any{"action": "auth.status"},
			},
			{
				"name":        "auth.logout",
				"description": "Revoke the current CLI token and remove local credentials.",
				"example":     map[string]any{"action": "auth.logout"},
			},
			{
				"name":        "config.show",
				"description": "Show local CLI configuration without exposing the full token.",
				"example":     map[string]any{"action": "config.show"},
			},
			{
				"name":        "operations.list",
				"description": "List OpenAPI operations; optional tag and search filters are supported.",
				"example": map[string]any{
					"action": "operations.list",
					"tag":    "knowledge",
					"search": "page",
				},
			},
			{
				"name":        "operation.describe",
				"description": "Show request and response schema details for one operation_id.",
				"example": map[string]any{
					"action":       "operation.describe",
					"operation_id": "createPage",
				},
			},
			{
				"name":        "operation.call",
				"description": "Call one OpenAPI operation with path, query, headers, body, and files.",
				"example": map[string]any{
					"action":       "operation.call",
					"operation_id": "createPage",
					"body": map[string]any{
						"namespace":     "main",
						"title":         "Agent page",
						"language":      "zh-Hans",
						"content_model": "block-v1",
					},
				},
			},
			{
				"name":        "collaboration.run",
				"description": "Recover and send Yjs collaboration WebSocket messages for a page.",
				"example": map[string]any{
					"action":        "collaboration.run",
					"page_id":       "0198...",
					"client_id":     "0198...",
					"last_sequence": 0,
				},
			},
		},
		"operation_call": map[string]any{
			"coverage": "All embedded OpenAPI operations are callable through operation.call.",
			"discovery": []string{
				`{"action":"operations.list"}`,
				`{"action":"operations.list","tag":"pages"}`,
				`{"action":"operations.list","search":"revision"}`,
				`{"action":"operation.describe","operation_id":"createPage"}`,
			},
			"request_parts": []string{
				"path: values for URL path parameters such as {id}",
				"query: query string parameters",
				"headers: extra HTTP headers such as Idempotency-Key",
				"body: JSON request body or multipart form fields",
				"files: multipart file fields, where each value is a local path",
			},
			"validation": []string{
				"Unknown input fields are rejected before execution.",
				"Path, query, header, body, and multipart fields are validated against OpenAPI before the request is sent.",
				"JSON responses are validated against the response schema for the returned status code.",
			},
			"binary_response": "Non-JSON responses are returned as {\"encoding\":\"base64\",\"content\":\"...\",\"size_bytes\":123}.",
		},
		"collaboration": map[string]any{
			"action":        "collaboration.run",
			"purpose":       "Recover a page WorkingDocument and optionally send Yjs update, presence, or snapshot messages.",
			"binary_fields": "Yjs update and snapshot bytes must be base64 strings.",
			"message_types": []string{"update", "presence", "snapshot"},
		},
		"environment": []string{
			"ANBY_WIKI_BASE_URL",
			"ANBY_WIKI_TOKEN",
			"ANBY_WIKI_CONFIG",
		},
		"config_paths": map[string]string{
			"macos": "~/Library/Application Support/anby-wiki/cli.json",
			"linux": "~/.config/anby-wiki/cli.json",
		},
		"exit_codes": map[string]int{
			"success":               0,
			"remote_or_runtime_err": 1,
			"input_or_contract_err": 2,
		},
		"common_examples": []map[string]any{
			{
				"title": "Show version",
				"json":  map[string]any{"action": "version"},
			},
			{
				"title": "Show current session",
				"json":  map[string]any{"action": "auth.status"},
			},
			{
				"title": "List page APIs",
				"json":  map[string]any{"action": "operations.list", "tag": "pages"},
			},
			{
				"title": "Describe createPage",
				"json":  map[string]any{"action": "operation.describe", "operation_id": "createPage"},
			},
			{
				"title": "Create a page",
				"json": map[string]any{
					"action":       "operation.call",
					"operation_id": "createPage",
					"body": map[string]any{
						"namespace":     "main",
						"title":         "CLI Created Page",
						"language":      "zh-Hans",
						"content_model": "block-v1",
					},
				},
			},
			{
				"title": "Call an API with path parameters",
				"json": map[string]any{
					"action":       "operation.call",
					"operation_id": "getPage",
					"path":         map[string]any{"id": "0198..."},
				},
			},
			{
				"title": "Upload a file",
				"json": map[string]any{
					"action":       "operation.call",
					"operation_id": "createImportUploadJob",
					"headers":      map[string]any{"Idempotency-Key": "0198..."},
					"body": map[string]any{
						"title":      "Import source",
						"route_mode": "auto",
					},
					"files":           map[string]any{"file": "/absolute/path/source.pdf"},
					"timeout_seconds": 600,
				},
			},
		},
		"docs": "Docs/AgentCLI.md",
	})
}

func (a *App) listOperations(input Input) Result {
	tag := strings.ToLower(strings.TrimSpace(input.Tag))
	search := strings.ToLower(strings.TrimSpace(input.Search))
	items := []operationSummary{}
	for _, operation := range a.contract.List() {
		if tag != "" && !containsFold(operation.Tags, tag) {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{
			operation.ID, operation.Method, operation.Path,
			operation.Summary, strings.Join(operation.Tags, " "),
		}, " "))
		if search != "" && !strings.Contains(haystack, search) {
			continue
		}
		items = append(items, operationSummary{
			OperationID: operation.ID, Method: operation.Method,
			Path: operation.Path, Summary: operation.Summary,
			Tags: operation.Tags, CLIOnly: operation.CLIOnly,
		})
	}
	return success(input.Action, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func (a *App) describeOperation(input Input) (Result, int) {
	operation, err := a.contract.Describe(input.OperationID)
	if errors.Is(err, clicontract.ErrOperationNotFound) {
		return failure(input.Action, "operation_not_found", "unknown operation_id", nil), 2
	}
	if err != nil {
		return failure(input.Action, "contract_error", err.Error(), nil), 1
	}
	return success(input.Action, operation), 0
}

func (a *App) callOperation(ctx context.Context, input Input) (Result, int) {
	configPath, config, err := a.runtimeConfig(input)
	if err != nil {
		return failure(input.Action, "config_invalid", err.Error(), nil), 2
	}
	_ = configPath
	response, err := a.invoke(ctx, input, config)
	if err != nil {
		return invocationFailure(input.Action, err, nil)
	}
	return remoteResult(input.Action, input.OperationID, response)
}

func (a *App) exchange(ctx context.Context, input Input) (Result, int) {
	if strings.TrimSpace(input.Code) == "" {
		return failure(input.Action, "validation_failed", "code is required", nil), 2
	}
	configPath, config, err := a.runtimeConfig(input)
	if err != nil {
		return failure(input.Action, "config_invalid", err.Error(), nil), 2
	}
	if config.BaseURL == "" {
		return failure(input.Action, "validation_failed", "base_url is required", nil), 2
	}
	persist := input.Persist == nil || *input.Persist
	if persist {
		if err := ensureConfigWritable(configPath); err != nil {
			return failure(input.Action, "config_write_failed", err.Error(), nil), 1
		}
	}
	body, _ := json.Marshal(map[string]string{"code": strings.TrimSpace(input.Code)})
	call := Input{
		Action: "operation.call", OperationID: "exchangeCLIAuthCode",
		BaseURL: config.BaseURL, ConfigPath: configPath, Body: body,
		TimeoutSeconds: input.TimeoutSeconds,
	}
	response, err := a.invoke(ctx, call, Config{BaseURL: config.BaseURL})
	if err != nil {
		return invocationFailure(input.Action, err, nil)
	}
	if response.Status < 200 || response.Status >= 300 {
		return remoteResult(input.Action, call.OperationID, response)
	}
	payload, ok := response.Body.(map[string]any)
	if !ok {
		return failure(input.Action, "response_invalid", "exchange response is not an object", response.Body), 1
	}
	token, _ := payload["token"].(string)
	actorID, _ := payload["actor_id"].(string)
	displayName, _ := payload["display_name"].(string)
	tokenInfo, _ := payload["token_info"].(map[string]any)
	tokenPrefix, _ := tokenInfo["token_prefix"].(string)
	expiresAt, err := parseJSONTime(tokenInfo["expires_at"])
	if token == "" || actorID == "" || err != nil {
		return failure(input.Action, "response_invalid", "exchange response lacks token metadata", nil), 1
	}
	data := map[string]any{
		"actor_id": actorID, "display_name": displayName,
		"token_prefix": tokenPrefix, "expires_at": expiresAt,
		"persisted": persist,
	}
	if persist {
		config = Config{
			BaseURL: config.BaseURL, Token: token, TokenPrefix: tokenPrefix,
			ActorID: actorID, DisplayName: displayName, ExpiresAt: &expiresAt,
		}
		if err := saveConfig(configPath, config); err != nil {
			return failure(input.Action, "config_write_failed", err.Error(), nil), 1
		}
		data["config_path"] = configPath
	} else {
		data["token"] = token
	}
	result := success(input.Action, data)
	result.Meta = responseMeta(call.OperationID, response)
	return result, 0
}

func (a *App) authStatus(ctx context.Context, input Input) (Result, int) {
	configPath, config, err := a.runtimeConfig(input)
	if err != nil {
		return failure(input.Action, "config_invalid", err.Error(), nil), 2
	}
	_ = configPath
	response, err := a.invoke(ctx, Input{
		Action: "operation.call", OperationID: "getSession",
		BaseURL: config.BaseURL, TimeoutSeconds: input.TimeoutSeconds,
	}, config)
	if err != nil {
		return invocationFailure(input.Action, err, nil)
	}
	return remoteResult(input.Action, "getSession", response)
}

func (a *App) logout(ctx context.Context, input Input) (Result, int) {
	configPath, config, err := a.runtimeConfig(input)
	if err != nil {
		return failure(input.Action, "config_invalid", err.Error(), nil), 2
	}
	if config.Token == "" {
		return failure(input.Action, "not_authenticated", "no CLI token is configured", nil), 2
	}
	response, invokeErr := a.invoke(ctx, Input{
		Action: "operation.call", OperationID: "revokeCurrentCLIToken",
		BaseURL: config.BaseURL, TimeoutSeconds: input.TimeoutSeconds,
	}, config)
	var inputErr *cliInputError
	if errors.As(invokeErr, &inputErr) {
		return invocationFailure(input.Action, invokeErr, nil)
	}
	config.Token = ""
	config.TokenPrefix = ""
	config.ActorID = ""
	config.DisplayName = ""
	config.ExpiresAt = nil
	if err := saveConfig(configPath, config); err != nil {
		return failure(input.Action, "config_write_failed", err.Error(), nil), 1
	}
	if invokeErr != nil {
		return invocationFailure(input.Action, invokeErr, map[string]any{
			"local_credentials_removed": true,
		})
	}
	if response.Status != http.StatusNoContent &&
		response.Status != http.StatusUnauthorized {
		return remoteResult(input.Action, "revokeCurrentCLIToken", response)
	}
	result := success(input.Action, map[string]any{
		"revoked":                   response.Status == http.StatusNoContent,
		"local_credentials_removed": true,
		"config_path":               configPath,
	})
	result.Meta = responseMeta("revokeCurrentCLIToken", response)
	return result, 0
}

func (a *App) showConfig(input Input) (Result, int) {
	path, config, err := a.runtimeConfig(input)
	if err != nil {
		return failure(input.Action, "config_invalid", err.Error(), nil), 2
	}
	return success(input.Action, map[string]any{
		"config_path": path, "base_url": config.BaseURL,
		"authenticated": config.Token != "", "token_prefix": config.TokenPrefix,
		"actor_id": config.ActorID, "display_name": config.DisplayName,
		"expires_at": config.ExpiresAt,
	}), 0
}

func (a *App) invoke(
	ctx context.Context,
	input Input,
	config Config,
) (remoteResponse, error) {
	operation, err := a.contract.Describe(input.OperationID)
	if err != nil {
		return remoteResponse{}, asCLIInputError(err)
	}
	baseURL := strings.TrimSpace(input.BaseURL)
	if baseURL == "" {
		baseURL = config.BaseURL
	}
	baseURL, err = normalizeBaseURL(baseURL)
	if err != nil {
		return remoteResponse{}, asCLIInputError(err)
	}
	bodyValue, bodyPresent, err := decodeBody(input.Body)
	if err != nil {
		return remoteResponse{}, asCLIInputError(fmt.Errorf("body: %w", err))
	}
	contentType := ""
	validationBody := bodyValue
	if len(input.Files) > 0 {
		contentType = "multipart/form-data"
		object, ok := bodyValue.(map[string]any)
		if !bodyPresent {
			object = map[string]any{}
			bodyPresent = true
		} else if !ok {
			return remoteResponse{}, asCLIInputError(
				errors.New("body must be an object when files are present"),
			)
		}
		for field, path := range input.Files {
			if strings.TrimSpace(field) == "" || strings.TrimSpace(path) == "" {
				return remoteResponse{}, asCLIInputError(
					errors.New("file field and path must be non-empty"),
				)
			}
			object[field] = path
		}
		validationBody = object
	} else if bodyPresent {
		contentType = "application/json"
	}
	if err := a.contract.ValidateCall(input.OperationID, clicontract.Call{
		Path: input.Path, Query: input.Query, Headers: input.Headers,
		Body: validationBody, BodyPresent: bodyPresent, ContentType: contentType,
	}); err != nil {
		return remoteResponse{}, asCLIInputError(
			fmt.Errorf("request validation failed: %w", err),
		)
	}

	endpoint, err := operationURL(baseURL, operation, input.Path, input.Query)
	if err != nil {
		return remoteResponse{}, asCLIInputError(err)
	}
	var reader io.Reader
	if len(input.Files) > 0 {
		var actualType string
		reader, actualType, err = multipartBody(bodyValue, input.Files)
		if err != nil {
			return remoteResponse{}, asCLIInputError(err)
		}
		contentType = actualType
	} else if bodyPresent {
		reader = bytes.NewReader(input.Body)
	}
	request, err := http.NewRequestWithContext(ctx, operation.Method, endpoint, reader)
	if err != nil {
		return remoteResponse{}, asCLIInputError(err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if config.Token != "" {
		request.Header.Set("Authorization", "Bearer "+config.Token)
	}
	if err := setRequestHeaders(request.Header, input.Headers); err != nil {
		return remoteResponse{}, asCLIInputError(err)
	}
	request.Header.Set("Accept", "application/json, application/octet-stream;q=0.9")

	timeout, err := requestTimeout(input.TimeoutSeconds)
	if err != nil {
		return remoteResponse{}, asCLIInputError(err)
	}
	client := *a.client
	client.Timeout = timeout
	response, err := client.Do(request)
	if err != nil {
		return remoteResponse{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil {
		return remoteResponse{}, err
	}
	if len(raw) > maxResponse {
		return remoteResponse{}, fmt.Errorf("response exceeds %d bytes", maxResponse)
	}
	actualContentType := response.Header.Get("Content-Type")
	decoded, err := decodeResponseBody(raw, actualContentType)
	if err != nil {
		return remoteResponse{}, err
	}
	if isJSONContentType(actualContentType) && len(raw) > 0 {
		if err := a.contract.ValidateResponse(
			input.OperationID, response.StatusCode, actualContentType, decoded,
		); err != nil {
			return remoteResponse{}, fmt.Errorf("response validation failed: %w", err)
		}
	}
	return remoteResponse{
		Status: response.StatusCode, ContentType: actualContentType,
		RequestID: response.Header.Get("X-Request-ID"),
		Headers:   safeResponseHeaders(response.Header), Body: decoded,
	}, nil
}

func (a *App) runtimeConfig(input Input) (string, Config, error) {
	path, err := configPath(input.ConfigPath, a.getenv)
	if err != nil {
		return "", Config{}, err
	}
	config, err := loadConfig(path)
	if err != nil {
		return "", Config{}, err
	}
	if value := strings.TrimSpace(a.getenv("ANBY_WIKI_BASE_URL")); value != "" {
		config.BaseURL = value
	}
	if value := strings.TrimSpace(a.getenv("ANBY_WIKI_TOKEN")); value != "" {
		config.Token = value
		if config.TokenPrefix == "" {
			config.TokenPrefix = redactToken(value)
		}
	}
	if value := strings.TrimSpace(input.BaseURL); value != "" {
		config.BaseURL = value
	}
	if config.BaseURL != "" {
		config.BaseURL, err = normalizeBaseURL(config.BaseURL)
		if err != nil {
			return "", Config{}, err
		}
	}
	return path, config, nil
}

func success(action string, data any) Result {
	return Result{OK: true, Action: action, Data: data}
}

func failure(action, code, message string, details any) Result {
	return Result{
		OK: false, Action: action,
		Error: &Error{Code: code, Message: message, Details: details},
	}
}

func asCLIInputError(err error) error {
	if err == nil {
		return nil
	}
	return &cliInputError{err: err}
}

func invocationFailure(action string, err error, details any) (Result, int) {
	var inputErr *cliInputError
	if errors.As(err, &inputErr) {
		code := "validation_failed"
		message := err.Error()
		if errors.Is(err, clicontract.ErrOperationNotFound) {
			code = "operation_not_found"
			message = "unknown operation_id"
		}
		return failure(action, code, message, details), 2
	}
	return failure(action, "request_failed", err.Error(), details), 1
}

func remoteResult(
	action, operationID string,
	response remoteResponse,
) (Result, int) {
	meta := responseMeta(operationID, response)
	if response.Status >= 200 && response.Status < 400 {
		result := success(action, response.Body)
		result.Meta = meta
		return result, 0
	}
	code := "remote_error"
	message := http.StatusText(response.Status)
	if object, ok := response.Body.(map[string]any); ok {
		if value, ok := object["code"].(string); ok && value != "" {
			code = value
		}
		if value, ok := object["message"].(string); ok && value != "" {
			message = value
		}
	}
	result := failure(action, code, message, response.Body)
	result.Meta = meta
	return result, 1
}

func responseMeta(operationID string, response remoteResponse) *Meta {
	return &Meta{
		OperationID: operationID, HTTPStatus: response.Status,
		RequestID: response.RequestID, ContentType: response.ContentType,
		Headers: response.Headers,
	}
}

func decodeBody(raw json.RawMessage) (any, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, true, err
	}
	return value, true, nil
}

func decodeResponseBody(raw []byte, contentType string) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if isJSONContentType(contentType) {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode JSON response: %w", err)
		}
		return value, nil
	}
	return map[string]any{
		"encoding":   "base64",
		"content":    base64.StdEncoding.EncodeToString(raw),
		"size_bytes": len(raw),
	}, nil
}

func multipartBody(body any, files map[string]string) (io.Reader, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	object, _ := body.(map[string]any)
	keys := make([]string, 0, len(object))
	for key := range object {
		if _, file := files[key]; !file {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, err := multipartValue(object[key])
		if err != nil {
			return nil, "", fmt.Errorf("multipart field %s: %w", key, err)
		}
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}
	fileFields := make([]string, 0, len(files))
	for field := range files {
		fileFields = append(fileFields, field)
	}
	sort.Strings(fileFields)
	for _, field := range fileFields {
		path := files[field]
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("read file %s: %w", field, err)
		}
		header := make(textproto.MIMEHeader)
		header.Set(
			"Content-Disposition",
			fmt.Sprintf(
				`form-data; name=%q; filename=%q`,
				field, filepath.Base(path),
			),
		)
		contentType := mime.TypeByExtension(filepath.Ext(path))
		if contentType == "" {
			contentType = http.DetectContentType(content)
		}
		header.Set("Content-Type", contentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(content); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return bytes.NewReader(buffer.Bytes()), writer.FormDataContentType(), nil
}

func multipartValue(value any) (string, error) {
	switch item := value.(type) {
	case string:
		return item, nil
	case json.Number:
		return item.String(), nil
	case bool:
		return strconv.FormatBool(item), nil
	case nil:
		return "", nil
	default:
		encoded, err := json.Marshal(item)
		return string(encoded), err
	}
}

func operationURL(
	baseURL string,
	operation clicontract.Descriptor,
	pathValues, queryValues map[string]any,
) (string, error) {
	route := operation.Path
	for _, parameter := range operation.Parameters {
		if parameter.In != "path" {
			continue
		}
		value, ok := pathValues[parameter.Name]
		if !ok {
			continue
		}
		text, err := scalarString(value)
		if err != nil {
			return "", fmt.Errorf("path %s: %w", parameter.Name, err)
		}
		route = strings.ReplaceAll(
			route, "{"+parameter.Name+"}", url.PathEscape(text),
		)
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	relative, err := url.Parse(route)
	if err != nil {
		return "", err
	}
	endpoint := base.ResolveReference(relative)
	query := endpoint.Query()
	queryKeys := make([]string, 0, len(queryValues))
	for key := range queryValues {
		queryKeys = append(queryKeys, key)
	}
	sort.Strings(queryKeys)
	for _, key := range queryKeys {
		switch value := queryValues[key].(type) {
		case []any:
			for _, item := range value {
				text, err := scalarString(item)
				if err != nil {
					return "", fmt.Errorf("query %s: %w", key, err)
				}
				query.Add(key, text)
			}
		default:
			text, err := scalarString(value)
			if err != nil {
				return "", fmt.Errorf("query %s: %w", key, err)
			}
			query.Set(key, text)
		}
	}
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func setRequestHeaders(target http.Header, values map[string]any) error {
	for name, raw := range values {
		if strings.EqualFold(name, "Authorization") ||
			strings.EqualFold(name, "Cookie") ||
			strings.EqualFold(name, "Content-Type") {
			return fmt.Errorf("header %s is managed by the CLI", name)
		}
		switch value := raw.(type) {
		case []any:
			for _, item := range value {
				text, err := scalarString(item)
				if err != nil {
					return fmt.Errorf("header %s: %w", name, err)
				}
				target.Add(name, text)
			}
		default:
			text, err := scalarString(value)
			if err != nil {
				return fmt.Errorf("header %s: %w", name, err)
			}
			target.Set(name, text)
		}
	}
	return nil
}

func scalarString(value any) (string, error) {
	switch item := value.(type) {
	case string:
		return item, nil
	case json.Number:
		return item.String(), nil
	case float64:
		return strconv.FormatFloat(item, 'g', -1, 64), nil
	case bool:
		return strconv.FormatBool(item), nil
	default:
		return "", fmt.Errorf("expected scalar, got %T", value)
	}
}

func normalizeBaseURL(raw string) (string, error) {
	value, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || value.Host == "" ||
		(value.Scheme != "http" && value.Scheme != "https") {
		return "", errors.New("base_url must be an absolute http(s) URL")
	}
	if value.User != nil || value.RawQuery != "" || value.Fragment != "" {
		return "", errors.New("base_url must not contain credentials, query, or fragment")
	}
	value.Path = strings.TrimRight(value.Path, "/")
	return strings.TrimRight(value.String(), "/"), nil
}

func requestTimeout(seconds int) (time.Duration, error) {
	if seconds == 0 {
		return defaultTimeout, nil
	}
	timeout := time.Duration(seconds) * time.Second
	if timeout < time.Second || timeout > maxTimeout {
		return 0, errors.New("timeout_seconds must be between 1 and 600")
	}
	return timeout, nil
}

func safeResponseHeaders(headers http.Header) map[string][]string {
	result := map[string][]string{}
	for _, name := range []string{
		"Content-Type", "X-Request-ID", "ETag", "Cache-Control", "Location",
		"Retry-After", "X-RateLimit-Limit", "X-RateLimit-Remaining",
	} {
		if values := headers.Values(name); len(values) > 0 {
			result[name] = values
		}
	}
	return result
}

func isJSONContentType(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "application/json") ||
		strings.Contains(value, "+json")
}

func parseJSONTime(value any) (time.Time, error) {
	text, ok := value.(string)
	if !ok {
		return time.Time{}, errors.New("time is not a string")
	}
	return time.Parse(time.RFC3339Nano, text)
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func redactToken(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
