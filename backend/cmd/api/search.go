// 页面搜索 API handler（M2-T04）：编辑器引用选择器（cmdk）的后端，
// 活页面标题/别名 ILIKE 匹配，匿名可读，无需 X-Actor-ID。
package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/platform/httpx"
	wikisearch "github.com/anby/wiki/backend/internal/search"
)

// SearchAPI 搜索 API 依赖集合：可替换 SearchAdapter 与默认站点 ID。
type SearchAPI struct {
	search             wikisearch.SearchAdapter
	titleAliasFallback wikisearch.SearchAdapter
	wikiID             uuid.UUID
}

// NewSearchAPI 装配搜索 API。
func NewSearchAPI(adapter wikisearch.SearchAdapter, wikiID uuid.UUID) *SearchAPI {
	return &SearchAPI{search: adapter, wikiID: wikiID}
}

// WithTitleAliasFallback preserves synchronous title/alias lookup for
// unpublished pages. Full-text queries continue to read only the projection.
func (a *SearchAPI) WithTitleAliasFallback(adapter wikisearch.SearchAdapter) *SearchAPI {
	a.titleAliasFallback = adapter
	return a
}

// ---- 响应 DTO（与 contracts/openapi/openapi.yaml 对应，契约为准）----

// searchHitResponse 单条搜索结果（PageSearchHit）。
type searchHitResponse struct {
	ID           uuid.UUID  `json:"id"`
	DisplayTitle string     `json:"display_title"`
	Namespace    string     `json:"namespace"`
	Language     string     `json:"language"`
	EntityID     *uuid.UUID `json:"entity_id,omitempty"`
	EntityType   string     `json:"entity_type,omitempty"`
	MatchedOn    string     `json:"matched_on"`
	Highlight    string     `json:"highlight"`
	Score        float32    `json:"score"`
}

type searchFacetValueResponse struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type searchFacetsResponse struct {
	Namespaces  []searchFacetValueResponse `json:"namespaces"`
	Languages   []searchFacetValueResponse `json:"languages"`
	EntityTypes []searchFacetValueResponse `json:"entity_types"`
}

// pageSearchResponse 搜索端点响应（PageSearchResults）。
type pageSearchResponse struct {
	Items  []searchHitResponse  `json:"items"`
	Total  int                  `json:"total"`
	Mode   string               `json:"mode"`
	Facets searchFacetsResponse `json:"facets"`
}

type searchCapabilitiesResponse struct {
	Backend         string   `json:"backend"`
	Modes           []string `json:"modes"`
	DefaultMode     string   `json:"default_mode"`
	DefaultRatio    float64  `json:"default_semantic_ratio"`
	AggregationKeys []string `json:"aggregation_keys"`
}

// searchPages GET /api/v1/pages/search?q=&namespace=main&limit=10：
// q 空时返回空列表；namespace 缺省不过滤；limit 缺省 10、上限由领域服务截断。
func (a *SearchAPI) searchPages(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > wikisearch.MaxLimit {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "limit 必须是 1 到 50 的整数: "+raw)
			return
		}
		limit = n
	}

	offset, ok := parseSearchInteger(w, r, "offset")
	if !ok {
		return
	}
	fields, ok := parseSearchFields(w, r)
	if !ok {
		return
	}
	mode := wikisearch.Mode(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = wikisearch.ModeKeyword
	}
	semanticRatio := 0.0
	if raw := r.URL.Query().Get("semantic_ratio"); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || value < 0 || value > 1 {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest,
				"semantic_ratio 必须是 0 到 1 之间的数字: "+raw)
			return
		}
		semanticRatio = value
	}

	adapter := a.search
	if mode == wikisearch.ModeKeyword && a.titleAliasFallback != nil && titleAliasFieldsOnly(fields) {
		adapter = a.titleAliasFallback
	}
	result, err := adapter.Search(r.Context(), wikisearch.Query{
		Text:          r.URL.Query().Get("q"),
		WikiID:        a.wikiID,
		Namespace:     namespace,
		Language:      r.URL.Query().Get("language"),
		EntityType:    r.URL.Query().Get("entity_type"),
		Fields:        fields,
		Mode:          mode,
		SemanticRatio: semanticRatio,
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		if errors.Is(err, wikisearch.ErrNamespaceNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "namespace 不存在")
			return
		}
		if errors.Is(err, wikisearch.ErrInvalidMode) {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest,
				"mode 必须是 keyword、hybrid 或 semantic")
			return
		}
		if errors.Is(err, wikisearch.ErrSemanticUnavailable) {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed,
				"当前环境未配置语义搜索；请改用 keyword 模式")
			return
		}
		serviceError(w, r, err)
		return
	}

	resp := pageSearchResponse{
		Items: make([]searchHitResponse, 0, len(result.Hits)),
		Total: result.Total,
		Mode:  string(result.Mode),
		Facets: searchFacetsResponse{
			Namespaces:  searchFacetResponses(result.Facets.Namespaces),
			Languages:   searchFacetResponses(result.Facets.Languages),
			EntityTypes: searchFacetResponses(result.Facets.EntityTypes),
		},
	}
	for _, h := range result.Hits {
		resp.Items = append(resp.Items, searchHitResponse{
			ID:           h.PageID,
			DisplayTitle: h.DisplayTitle,
			Namespace:    h.Namespace,
			Language:     h.Language,
			EntityID:     h.EntityID,
			EntityType:   h.EntityType,
			MatchedOn:    string(h.MatchedOn),
			Highlight:    h.Highlight,
			Score:        h.Score,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *SearchAPI) capabilities(w http.ResponseWriter, _ *http.Request) {
	capabilities := a.search.Capabilities()
	modes := make([]string, 0, len(capabilities.Modes))
	for _, mode := range capabilities.Modes {
		modes = append(modes, string(mode))
	}
	httpx.WriteJSON(w, http.StatusOK, searchCapabilitiesResponse{
		Backend:         capabilities.Backend,
		Modes:           modes,
		DefaultMode:     string(capabilities.DefaultMode),
		DefaultRatio:    capabilities.DefaultRatio,
		AggregationKeys: capabilities.AggregationKeys,
	})
}

func searchFacetResponses(values []wikisearch.FacetValue) []searchFacetValueResponse {
	result := make([]searchFacetValueResponse, 0, len(values))
	for _, value := range values {
		result = append(result, searchFacetValueResponse{Value: value.Value, Count: value.Count})
	}
	return result
}

func titleAliasFieldsOnly(fields []wikisearch.Field) bool {
	if len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		if field != wikisearch.FieldTitle && field != wikisearch.FieldAlias {
			return false
		}
	}
	return true
}

func parseSearchInteger(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, name+" 必须是非负整数: "+raw)
		return 0, false
	}
	return value, true
}

func parseSearchFields(w http.ResponseWriter, r *http.Request) ([]wikisearch.Field, bool) {
	rawFields := r.URL.Query()["fields"]
	if len(rawFields) == 0 {
		return nil, true
	}
	fields := make([]wikisearch.Field, 0, len(rawFields))
	seen := make(map[wikisearch.Field]struct{})
	for _, raw := range rawFields {
		for _, value := range strings.Split(raw, ",") {
			field := wikisearch.Field(strings.TrimSpace(value))
			switch field {
			case wikisearch.FieldTitle, wikisearch.FieldAlias, wikisearch.FieldBody, wikisearch.FieldEntity:
				if _, exists := seen[field]; !exists {
					seen[field] = struct{}{}
					fields = append(fields, field)
				}
			default:
				httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "不支持的搜索字段: "+value)
				return nil, false
			}
		}
	}
	return fields, true
}
