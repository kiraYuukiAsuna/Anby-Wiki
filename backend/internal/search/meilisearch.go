package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultMeiliTaskPollInterval = 250 * time.Millisecond
	defaultMeiliTaskTimeout      = 10 * time.Minute
	maxMeiliResponseBytes        = 4 << 20
)

// MeilisearchConfig configures the official HTTP API client without adding an
// SDK dependency to the domain boundary.
type MeilisearchConfig struct {
	BaseURL          string
	APIKey           string
	Index            string
	HTTPClient       *http.Client
	TaskPollInterval time.Duration
	TaskTimeout      time.Duration
	SemanticEmbedder string
	// EmbedderConfig is sent only to the index settings endpoint. It may
	// contain a provider key and must never be logged or copied into a search
	// document.
	EmbedderConfig map[string]any
}

// MeilisearchAdapter implements keyword, hybrid, and semantic search.
type MeilisearchAdapter struct {
	baseURL          *url.URL
	apiKey           string
	index            string
	client           *http.Client
	taskPollInterval time.Duration
	taskTimeout      time.Duration
	semanticEmbedder string
	embedderConfig   map[string]any
	namespaceExists  func(context.Context, uuid.UUID, string) (bool, error)
}

func NewMeilisearchAdapter(cfg MeilisearchConfig) (*MeilisearchAdapter, error) {
	baseURL, err := url.Parse(strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" ||
		(baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return nil, fmt.Errorf("search: invalid Meilisearch base URL")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("search: Meilisearch base URL must not contain credentials, query, or fragment")
	}
	if strings.TrimSpace(cfg.Index) == "" {
		return nil, fmt.Errorf("search: Meilisearch index is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	pollInterval := cfg.TaskPollInterval
	if pollInterval <= 0 {
		pollInterval = defaultMeiliTaskPollInterval
	}
	taskTimeout := cfg.TaskTimeout
	if taskTimeout <= 0 {
		taskTimeout = defaultMeiliTaskTimeout
	}
	embedder := strings.TrimSpace(cfg.SemanticEmbedder)
	if embedder == "" && len(cfg.EmbedderConfig) > 0 {
		return nil, fmt.Errorf("search: semantic embedder name is required")
	}
	if embedder != "" && len(cfg.EmbedderConfig) == 0 {
		return nil, fmt.Errorf("search: semantic embedder configuration is required")
	}
	return &MeilisearchAdapter{
		baseURL:          baseURL,
		apiKey:           cfg.APIKey,
		index:            cfg.Index,
		client:           client,
		taskPollInterval: pollInterval,
		taskTimeout:      taskTimeout,
		semanticEmbedder: embedder,
		embedderConfig:   cfg.EmbedderConfig,
	}, nil
}

func (a *MeilisearchAdapter) Capabilities() Capabilities {
	capabilities := keywordCapabilities(BackendMeilisearch)
	if a.semanticEmbedder != "" {
		capabilities.Modes = []Mode{ModeKeyword, ModeHybrid, ModeSemantic}
		capabilities.DefaultMode = ModeHybrid
	}
	return capabilities
}

// EnsureIndex creates the index if needed and applies deterministic settings.
func (a *MeilisearchAdapter) EnsureIndex(ctx context.Context) error {
	status, err := a.do(ctx, http.MethodGet, "/indexes/"+url.PathEscape(a.index), nil, nil)
	if err != nil && status != http.StatusNotFound {
		return fmt.Errorf("search: inspect Meilisearch index: %w", err)
	}
	if status == http.StatusNotFound {
		var task enqueuedTask
		status, err = a.do(ctx, http.MethodPost, "/indexes", map[string]any{
			"uid": a.index, "primaryKey": "page_id",
		}, &task)
		if err != nil && status != http.StatusConflict {
			return fmt.Errorf("search: create Meilisearch index: %w", err)
		}
		if status != http.StatusConflict {
			if err := a.waitTask(ctx, task.TaskUID); err != nil {
				return fmt.Errorf("search: create Meilisearch index: %w", err)
			}
		}
	}

	settings := map[string]any{
		"searchableAttributes": []string{
			"display_title", "normalized_title", "aliases", "body", "entity_terms",
		},
		"displayedAttributes": []string{
			"page_id", "wiki_id", "namespace", "language", "source_revision_id",
			"display_title", "normalized_title", "aliases", "body", "entity_id",
			"entity_type", "entity_terms",
		},
		"filterableAttributes": []string{"wiki_id", "namespace", "language", "entity_type"},
		"sortableAttributes":   []string{},
		"pagination":           map[string]int{"maxTotalHits": 100000},
		"faceting":             map[string]int{"maxValuesPerFacet": 100},
	}
	if a.semanticEmbedder != "" {
		settings["embedders"] = map[string]any{
			a.semanticEmbedder: a.embedderConfig,
		}
	}
	var task enqueuedTask
	if _, err := a.do(ctx, http.MethodPatch,
		"/indexes/"+url.PathEscape(a.index)+"/settings", settings, &task); err != nil {
		return fmt.Errorf("search: configure Meilisearch index: %w", err)
	}
	if err := a.waitTask(ctx, task.TaskUID); err != nil {
		return fmt.Errorf("search: configure Meilisearch index: %w", err)
	}
	return nil
}

func (a *MeilisearchAdapter) Index(ctx context.Context, doc SearchDocument) error {
	if err := validateDocument(doc); err != nil {
		return err
	}
	var task enqueuedTask
	path := "/indexes/" + url.PathEscape(a.index) + "/documents?primaryKey=page_id"
	if _, err := a.do(ctx, http.MethodPut, path, []meiliDocument{toMeiliDocument(doc)}, &task); err != nil {
		return fmt.Errorf("search: index Meilisearch page %s: %w", doc.PageID, err)
	}
	if err := a.waitTask(ctx, task.TaskUID); err != nil {
		return fmt.Errorf("search: index Meilisearch page %s: %w", doc.PageID, err)
	}
	return nil
}

func (a *MeilisearchAdapter) Delete(ctx context.Context, pageID uuid.UUID) error {
	if pageID == uuid.Nil {
		return ErrInvalidDocument
	}
	var task enqueuedTask
	path := "/indexes/" + url.PathEscape(a.index) + "/documents/" + url.PathEscape(pageID.String())
	if _, err := a.do(ctx, http.MethodDelete, path, nil, &task); err != nil {
		return fmt.Errorf("search: delete Meilisearch page %s: %w", pageID, err)
	}
	if err := a.waitTask(ctx, task.TaskUID); err != nil {
		return fmt.Errorf("search: delete Meilisearch page %s: %w", pageID, err)
	}
	return nil
}

func (a *MeilisearchAdapter) Rebuild(ctx context.Context, documents []SearchDocument) error {
	for _, doc := range documents {
		if err := validateDocument(doc); err != nil {
			return err
		}
	}
	var task enqueuedTask
	path := "/indexes/" + url.PathEscape(a.index) + "/documents"
	if _, err := a.do(ctx, http.MethodDelete, path, nil, &task); err != nil {
		return fmt.Errorf("search: clear Meilisearch index: %w", err)
	}
	if err := a.waitTask(ctx, task.TaskUID); err != nil {
		return fmt.Errorf("search: clear Meilisearch index: %w", err)
	}
	if len(documents) == 0 {
		return nil
	}
	const batchSize = 1000
	for start := 0; start < len(documents); start += batchSize {
		end := min(start+batchSize, len(documents))
		payload := make([]meiliDocument, 0, end-start)
		for _, doc := range documents[start:end] {
			payload = append(payload, toMeiliDocument(doc))
		}
		if _, err := a.do(ctx, http.MethodPut, path+"?primaryKey=page_id", payload, &task); err != nil {
			return fmt.Errorf("search: rebuild Meilisearch index: %w", err)
		}
		if err := a.waitTask(ctx, task.TaskUID); err != nil {
			return fmt.Errorf("search: rebuild Meilisearch index: %w", err)
		}
	}
	return nil
}

func (a *MeilisearchAdapter) Search(ctx context.Context, query Query) (Result, error) {
	query = normalizeQuery(query)
	if err := validateQuery(query); err != nil {
		return Result{}, err
	}
	if query.Mode != ModeKeyword && a.semanticEmbedder == "" {
		return Result{}, ErrSemanticUnavailable
	}
	if query.Namespace != "" && a.namespaceExists != nil {
		exists, err := a.namespaceExists(ctx, query.WikiID, query.Namespace)
		if err != nil {
			return Result{}, fmt.Errorf("search: validate namespace: %w", err)
		}
		if !exists {
			return Result{}, ErrNamespaceNotFound
		}
	}
	if query.Text == "" {
		return emptyResult(query.Mode), nil
	}
	attributes := meiliSearchableAttributes(query.Fields)
	body := map[string]any{
		"q":                     query.Text,
		"limit":                 query.Limit,
		"offset":                query.Offset,
		"attributesToSearchOn":  attributes,
		"attributesToHighlight": attributes,
		"highlightPreTag":       "[[",
		"highlightPostTag":      "]]",
		"showRankingScore":      true,
		"facets":                []string{"namespace", "language", "entity_type"},
	}
	if query.Mode != ModeKeyword {
		body["hybrid"] = map[string]any{
			"embedder":      a.semanticEmbedder,
			"semanticRatio": query.SemanticRatio,
		}
	}
	if filter := meiliFilter(query); filter != "" {
		body["filter"] = filter
	}
	if containsString(attributes, "body") {
		body["attributesToCrop"] = []string{"body"}
		body["cropLength"] = 24
		body["cropMarker"] = "..."
	}
	var response meiliSearchResponse
	path := "/indexes/" + url.PathEscape(a.index) + "/search"
	if _, err := a.do(ctx, http.MethodPost, path, body, &response); err != nil {
		return Result{}, fmt.Errorf("search: query Meilisearch: %w", err)
	}
	result := emptyResult(query.Mode)
	result.Total = response.EstimatedTotalHits
	result.Facets = fromMeiliFacets(response.FacetDistribution)
	result.Hits = make([]Hit, 0, len(response.Hits))
	for _, raw := range response.Hits {
		pageID, err := uuid.Parse(raw.PageID)
		if err != nil {
			return Result{}, fmt.Errorf("search: invalid page id from Meilisearch")
		}
		var entityID *uuid.UUID
		if raw.EntityID != nil && *raw.EntityID != "" {
			parsed, err := uuid.Parse(*raw.EntityID)
			if err != nil {
				return Result{}, fmt.Errorf("search: invalid entity id from Meilisearch")
			}
			entityID = &parsed
		}
		field, highlight := bestMeiliHighlight(raw.Formatted, attributes, raw.DisplayTitle)
		result.Hits = append(result.Hits, Hit{
			PageID: pageID, DisplayTitle: raw.DisplayTitle, Namespace: raw.Namespace,
			Language: raw.Language, EntityID: entityID, EntityType: raw.EntityType,
			MatchedOn: field, Highlight: highlight, Score: raw.RankingScore,
		})
	}
	return result, nil
}

type meiliDocument struct {
	PageID           string   `json:"page_id"`
	WikiID           string   `json:"wiki_id"`
	Namespace        string   `json:"namespace"`
	Language         string   `json:"language"`
	SourceRevisionID string   `json:"source_revision_id"`
	DisplayTitle     string   `json:"display_title"`
	NormalizedTitle  string   `json:"normalized_title"`
	Aliases          []string `json:"aliases"`
	Body             string   `json:"body"`
	EntityID         *string  `json:"entity_id"`
	EntityType       string   `json:"entity_type"`
	EntityTerms      []string `json:"entity_terms"`
}

func toMeiliDocument(doc SearchDocument) meiliDocument {
	var entityID *string
	if doc.EntityID != nil {
		value := doc.EntityID.String()
		entityID = &value
	}
	return meiliDocument{
		PageID: doc.PageID.String(), WikiID: doc.WikiID.String(), Namespace: doc.Namespace,
		Language: doc.Language, SourceRevisionID: doc.SourceRevisionID.String(),
		DisplayTitle: doc.DisplayTitle, NormalizedTitle: doc.NormalizedTitle,
		Aliases: nonNilStrings(doc.Aliases), Body: doc.Body, EntityID: entityID,
		EntityType: doc.EntityType, EntityTerms: nonNilStrings(doc.EntityTerms),
	}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func meiliSearchableAttributes(fields []Field) []string {
	title, alias, body, entity := selectedFields(fields)
	attributes := make([]string, 0, 5)
	if title {
		attributes = append(attributes, "display_title", "normalized_title")
	}
	if alias {
		attributes = append(attributes, "aliases")
	}
	if body {
		attributes = append(attributes, "body")
	}
	if entity {
		attributes = append(attributes, "entity_terms")
	}
	return attributes
}

func meiliFilter(query Query) string {
	filters := make([]string, 0, 4)
	if query.WikiID != uuid.Nil {
		filters = append(filters, "wiki_id = "+strconv.Quote(query.WikiID.String()))
	}
	for field, value := range map[string]string{
		"namespace": query.Namespace, "language": query.Language, "entity_type": query.EntityType,
	} {
		if value != "" {
			filters = append(filters, field+" = "+strconv.Quote(value))
		}
	}
	return strings.Join(filters, " AND ")
}

type meiliSearchResponse struct {
	Hits               []meiliHit                `json:"hits"`
	EstimatedTotalHits int                       `json:"estimatedTotalHits"`
	FacetDistribution  map[string]map[string]int `json:"facetDistribution"`
}

type meiliHit struct {
	PageID       string                     `json:"page_id"`
	DisplayTitle string                     `json:"display_title"`
	Namespace    string                     `json:"namespace"`
	Language     string                     `json:"language"`
	EntityID     *string                    `json:"entity_id"`
	EntityType   string                     `json:"entity_type"`
	RankingScore float32                    `json:"_rankingScore"`
	Formatted    map[string]json.RawMessage `json:"_formatted"`
}

func fromMeiliFacets(distribution map[string]map[string]int) Facets {
	result := emptyResult(ModeKeyword).Facets
	result.Namespaces = sortedFacetValues(distribution["namespace"])
	result.Languages = sortedFacetValues(distribution["language"])
	result.EntityTypes = sortedFacetValues(distribution["entity_type"])
	return result
}

func sortedFacetValues(values map[string]int) []FacetValue {
	result := make([]FacetValue, 0, len(values))
	for value, count := range values {
		if value != "" {
			result = append(result, FacetValue{Value: value, Count: count})
		}
	}
	sortFacetValues(result)
	return result
}

func sortFacetValues(values []FacetValue) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0; j-- {
			if values[j-1].Count > values[j].Count ||
				(values[j-1].Count == values[j].Count && values[j-1].Value <= values[j].Value) {
				break
			}
			values[j-1], values[j] = values[j], values[j-1]
		}
	}
}

func bestMeiliHighlight(formatted map[string]json.RawMessage, attributes []string, fallback string) (Field, string) {
	for _, attribute := range attributes {
		value := formattedValue(formatted[attribute])
		if strings.Contains(value, "[[") {
			return meiliField(attribute), value
		}
	}
	for _, attribute := range attributes {
		if value := formattedValue(formatted[attribute]); value != "" {
			return meiliField(attribute), value
		}
	}
	return FieldTitle, fallback
}

func formattedValue(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	var values []string
	if json.Unmarshal(raw, &values) == nil {
		return strings.Join(values, " ")
	}
	return ""
}

func meiliField(attribute string) Field {
	switch attribute {
	case "aliases":
		return FieldAlias
	case "body":
		return FieldBody
	case "entity_terms":
		return FieldEntity
	default:
		return FieldTitle
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type enqueuedTask struct {
	TaskUID int64 `json:"taskUid"`
}

type meiliTask struct {
	Status string      `json:"status"`
	Error  *meiliError `json:"error"`
}

type meiliError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

func (a *MeilisearchAdapter) waitTask(ctx context.Context, uid int64) error {
	if a.taskTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.taskTimeout)
		defer cancel()
	}
	for {
		var task meiliTask
		if _, err := a.do(ctx, http.MethodGet, "/tasks/"+strconv.FormatInt(uid, 10), nil, &task); err != nil {
			return err
		}
		switch task.Status {
		case "succeeded":
			return nil
		case "failed", "canceled":
			if task.Error != nil {
				return fmt.Errorf("Meilisearch task %d %s: %s", uid, task.Status, task.Error.Code)
			}
			return fmt.Errorf("Meilisearch task %d %s", uid, task.Status)
		}
		timer := time.NewTimer(a.taskPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (a *MeilisearchAdapter) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	endpoint := *a.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	if index := strings.IndexByte(path, '?'); index >= 0 {
		endpoint.Path = strings.TrimRight(a.baseURL.Path, "/") + path[:index]
		endpoint.RawQuery = path[index+1:]
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxMeiliResponseBytes))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr meiliError
		if json.Unmarshal(raw, &apiErr) == nil && apiErr.Code != "" {
			return resp.StatusCode, fmt.Errorf("HTTP %d: %s", resp.StatusCode, apiErr.Code)
		}
		return resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

var _ SearchAdapter = (*MeilisearchAdapter)(nil)
