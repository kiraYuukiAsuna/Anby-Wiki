package component

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/platform/db"
)

const (
	// RendererEntityClaimInfobox 是 M9-T03 内置信息框渲染器的白名单引用。
	RendererEntityClaimInfobox = "builtin.entity_claim_infobox"
	// BuiltinArticleInfoboxVersion is the frozen version seeded by the initial
	// schema. Import proposals may safely embed this stable dependency.
	BuiltinArticleInfoboxVersion = 1
)

// BuiltinArticleInfoboxID is deliberately stable across fresh installations.
var BuiltinArticleInfoboxID = uuid.MustParse(
	"00000000-0000-7000-8000-000000000901",
)

type infoboxConfig struct {
	Title        string   `json:"title"`
	Language     string   `json:"language"`
	PropertyKeys []string `json:"property_keys"`
}

type infoboxRenderer struct {
	q db.Querier
}

// NewKnowledgeRegistry 注册只存在于进程内的 Entity/Claim 信息框实现。
func NewKnowledgeRegistry(q db.Querier) *Registry {
	registry := NewDefaultRegistry()
	if err := registry.Register(RendererEntityClaimInfobox, &infoboxRenderer{q: q}); err != nil {
		panic(err)
	}
	return registry
}

// Render 仅用于满足 Renderer；信息框必须携带 ComponentBlock.entity_id。
func (r *infoboxRenderer) Render(context.Context, json.RawMessage) (string, error) {
	return "", fmt.Errorf("%w: entity_id is required", ErrInvalidProps)
}

func (r *infoboxRenderer) RenderEntity(
	ctx context.Context, entityID uuid.UUID, displayConfig json.RawMessage,
) (string, error) {
	config, err := parseInfoboxConfig(displayConfig)
	if err != nil {
		return "", err
	}
	entity, err := resolveInfoboxEntity(ctx, r.q, entityID, config.Language)
	if err != nil {
		return "", err
	}
	rows, err := resolveInfoboxClaims(ctx, r.q, entityID, config)
	if err != nil {
		return "", err
	}
	return renderEntityInfobox(entityID, entity, rows, config), nil
}

type infoboxEntity struct {
	label    string
	typeKey  string
	typeName string
}

func renderEntityInfobox(
	entityID uuid.UUID,
	entity infoboxEntity,
	rows []infoboxClaim,
	config infoboxConfig,
) string {
	title := config.Title
	if title == "" {
		title = entity.label
	}
	groups := groupInfoboxClaims(rows, config.PropertyKeys, entity.typeKey)
	var out strings.Builder
	fmt.Fprintf(&out,
		`<aside class="entity-infobox" data-component-renderer="entity-claim-infobox" data-entity-id="%s" data-entity-type="%s">`,
		html.EscapeString(entityID.String()), html.EscapeString(entity.typeKey))
	out.WriteString(`<header class="entity-infobox-header">`)
	if entity.typeName != "" {
		fmt.Fprintf(&out, `<p class="entity-infobox-type">%s</p>`, html.EscapeString(entity.typeName))
	}
	fmt.Fprintf(&out, `<h2>%s</h2></header>`, html.EscapeString(title))
	if len(groups) == 0 {
		out.WriteString(`<p class="entity-infobox-empty">暂无可展示的结构化事实</p>`)
		out.WriteString(`</aside>`)
		return out.String()
	}
	out.WriteString(`<dl>`)
	for _, group := range groups {
		fmt.Fprintf(&out,
			`<div data-property-key="%s"><dt>%s</dt><dd>`,
			html.EscapeString(group.propertyKey), html.EscapeString(group.propertyName))
		for index, row := range group.rows {
			if index > 0 {
				out.WriteString(`<span class="entity-infobox-separator" aria-hidden="true">、</span>`)
			}
			fmt.Fprintf(&out,
				`<span class="entity-infobox-value" data-claim-id="%s" data-verification-status="%s">`,
				html.EscapeString(row.claimID.String()), html.EscapeString(row.verificationStatus))
			if row.targetEntityID != nil {
				fmt.Fprintf(&out, `<a href="/entities/%s">%s</a>`,
					html.EscapeString(row.targetEntityID.String()), html.EscapeString(row.value))
			} else {
				out.WriteString(html.EscapeString(row.value))
			}
			out.WriteString(`</span>`)
		}
		out.WriteString(`</dd></div>`)
	}
	out.WriteString(`</dl>`)
	if summary := infoboxVerificationSummary(groups); summary != "" {
		fmt.Fprintf(&out, `<footer class="entity-infobox-verification">%s</footer>`, html.EscapeString(summary))
	}
	out.WriteString(`</aside>`)
	return out.String()
}

type infoboxClaim struct {
	claimID            uuid.UUID
	propertyKey        string
	propertyName       string
	value              string
	targetEntityID     *uuid.UUID
	verificationStatus string
}

type infoboxClaimGroup struct {
	propertyKey  string
	propertyName string
	rows         []infoboxClaim
}

func parseInfoboxConfig(raw json.RawMessage) (infoboxConfig, error) {
	var config infoboxConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return config, fmt.Errorf("%w: display_config must be an object", ErrInvalidProps)
	}
	return config, nil
}

func resolveInfoboxEntity(
	ctx context.Context, q db.Querier, entityID uuid.UUID, language string,
) (infoboxEntity, error) {
	var (
		result       infoboxEntity
		canonicalKey string
		label        *string
	)
	err := q.QueryRow(ctx, `
		WITH RECURSIVE entity_chain AS (
			SELECT e.id, e.entity_type_id, e.canonical_key, e.status, e.merged_into_entity_id,
			       0 AS depth, ARRAY[e.id] AS path
			FROM entity e
			WHERE e.id = $1
			UNION ALL
			SELECT target.id, target.entity_type_id, target.canonical_key, target.status,
			       target.merged_into_entity_id, chain.depth + 1,
			       chain.path || target.id
			FROM entity_chain chain
			JOIN entity target ON target.id = chain.merged_into_entity_id
			WHERE chain.status = 'merged'
			  AND chain.depth < 63
			  AND NOT target.id = ANY(chain.path)
		)
		SELECT e.canonical_key, et.type_key, et.name,
			(SELECT el.label FROM entity_label el
			 WHERE el.entity_id = e.id AND el.is_primary
			 ORDER BY CASE
			   WHEN $2 <> '' AND el.language = $2 THEN 0
			   WHEN el.language = 'und' THEN 1
			   ELSE 2
			 END, el.language, el.label LIMIT 1)
		FROM entity_chain e
		JOIN entity_type et ON et.id = e.entity_type_id
		WHERE e.status <> 'deleted'
		ORDER BY (e.status = 'active') DESC, e.depth DESC
		LIMIT 1`, entityID, language).Scan(
		&canonicalKey, &result.typeKey, &result.typeName, &label,
	)
	if err != nil {
		return result, fmt.Errorf("component: resolve entity %s label: %w", entityID, err)
	}
	if label != nil {
		result.label = *label
	} else {
		result.label = displayCanonicalKey(canonicalKey)
	}
	return result, nil
}

func resolveInfoboxClaims(
	ctx context.Context, q db.Querier, entityID uuid.UUID, config infoboxConfig,
) ([]infoboxClaim, error) {
	rows, err := q.Query(ctx, `
		WITH RECURSIVE entity_chain AS (
			SELECT e.id, e.status, e.merged_into_entity_id,
			       0 AS depth, ARRAY[e.id] AS path
			FROM entity e
			WHERE e.id = $1
			UNION ALL
			SELECT target.id, target.status, target.merged_into_entity_id,
			       chain.depth + 1, chain.path || target.id
			FROM entity_chain chain
			JOIN entity target ON target.id = chain.merged_into_entity_id
			WHERE chain.status = 'merged'
			  AND chain.depth < 63
			  AND NOT target.id = ANY(chain.path)
		),
		resolved_entity AS (
			SELECT id
			FROM entity_chain
			WHERE status <> 'deleted'
			ORDER BY (status = 'active') DESC, depth DESC
			LIMIT 1
		)
		SELECT c.id, p.property_key, p.name, c.value_type, c.value_json, target.id,
		       c.verification_status,
		       target.canonical_key,
		       (SELECT el.label FROM entity_label el
		          WHERE el.entity_id = target.id AND el.is_primary
		          ORDER BY CASE
		            WHEN $3 <> '' AND el.language = $3 THEN 0
		            WHEN el.language = 'und' THEN 1
		            ELSE 2
		          END, el.language, el.label LIMIT 1)
		FROM claim c
		JOIN property p ON p.id = c.property_id
		LEFT JOIN LATERAL (
			WITH RECURSIVE target_chain AS (
				SELECT e.id, e.canonical_key, e.status,
				       e.merged_into_entity_id, 0 AS depth,
				       ARRAY[e.id] AS path
				FROM entity e
				WHERE e.id = c.target_entity_id
				UNION ALL
				SELECT next.id, next.canonical_key, next.status,
				       next.merged_into_entity_id, chain.depth + 1,
				       chain.path || next.id
				FROM target_chain chain
				JOIN entity next ON next.id = chain.merged_into_entity_id
				WHERE chain.status = 'merged'
				  AND chain.depth < 63
				  AND NOT next.id = ANY(chain.path)
			)
			SELECT id, canonical_key
			FROM target_chain
			WHERE status <> 'deleted'
			ORDER BY (status = 'active') DESC, depth DESC
			LIMIT 1
		) target ON true
		WHERE c.subject_entity_id = (SELECT id FROM resolved_entity)
		  AND c.status = 'published'
		  AND c.rank <> 'deprecated'
		  AND (target.id IS NULL OR target.id <> (SELECT id FROM resolved_entity))
		  AND (cardinality($2::text[]) = 0 OR p.property_key = ANY($2::text[]))
		ORDER BY CASE c.rank WHEN 'preferred' THEN 0 ELSE 1 END,
		         p.name, c.created_at, c.id`,
		entityID, config.PropertyKeys, config.Language)
	if err != nil {
		return nil, fmt.Errorf("component: query entity %s claims: %w", entityID, err)
	}
	defer rows.Close()

	var result []infoboxClaim
	for rows.Next() {
		var (
			row         infoboxClaim
			valueType   string
			valueJSON   []byte
			targetID    *uuid.UUID
			targetKey   *string
			targetLabel *string
		)
		if err := rows.Scan(
			&row.claimID, &row.propertyKey, &row.propertyName, &valueType, &valueJSON, &targetID,
			&row.verificationStatus, &targetKey, &targetLabel,
		); err != nil {
			return nil, fmt.Errorf("component: scan infobox claim: %w", err)
		}
		row.value, err = infoboxValueText(valueType, valueJSON, targetID, targetKey, targetLabel)
		if err != nil {
			return nil, err
		}
		row.targetEntityID = targetID
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("component: iterate infobox claims: %w", err)
	}
	return result, nil
}

func infoboxValueText(
	valueType string,
	raw json.RawMessage,
	targetID *uuid.UUID,
	targetCanonicalKey *string,
	targetLabel *string,
) (string, error) {
	if valueType == "entity" {
		if targetLabel != nil {
			return *targetLabel, nil
		}
		if targetCanonicalKey != nil {
			return displayCanonicalKey(*targetCanonicalKey), nil
		}
		if targetID != nil {
			return targetID.String(), nil
		}
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("component: decode claim value: %w", err)
	}
	switch typed := value.(type) {
	case string:
		return typed, nil
	case float64, bool:
		return fmt.Sprint(typed), nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", fmt.Errorf("component: encode claim value: %w", err)
		}
		return string(encoded), nil
	}
}

func displayCanonicalKey(value string) string {
	value = strings.TrimSpace(value)
	if _, label, found := strings.Cut(value, ":"); found && strings.TrimSpace(label) != "" {
		return strings.TrimSpace(label)
	}
	return value
}

func groupInfoboxClaims(
	rows []infoboxClaim,
	propertyKeys []string,
	entityType string,
) []infoboxClaimGroup {
	groups := make([]infoboxClaimGroup, 0, len(rows))
	indexes := make(map[string]int, len(rows))
	seenValues := make(map[string]map[string]bool, len(rows))
	for _, row := range rows {
		index, exists := indexes[row.propertyKey]
		if !exists {
			index = len(groups)
			indexes[row.propertyKey] = index
			groups = append(groups, infoboxClaimGroup{
				propertyKey: row.propertyKey, propertyName: row.propertyName,
			})
			seenValues[row.propertyKey] = map[string]bool{}
		}
		valueKey := strings.ToLower(strings.TrimSpace(row.value))
		if row.targetEntityID != nil {
			valueKey = row.targetEntityID.String()
		}
		if seenValues[row.propertyKey][valueKey] {
			continue
		}
		seenValues[row.propertyKey][valueKey] = true
		groups[index].rows = append(groups[index].rows, row)
	}
	order := make(map[string]int, len(propertyKeys))
	for index, key := range propertyKeys {
		order[key] = index
	}
	sort.SliceStable(groups, func(i, j int) bool {
		left := infoboxPropertyPriority(groups[i].propertyKey, entityType, order)
		right := infoboxPropertyPriority(groups[j].propertyKey, entityType, order)
		if left != right {
			return left < right
		}
		return groups[i].propertyName < groups[j].propertyName
	})
	for index := range groups {
		sort.SliceStable(groups[index].rows, func(i, j int) bool {
			return strings.ToLower(groups[index].rows[i].value) < strings.ToLower(groups[index].rows[j].value)
		})
	}
	return groups
}

func infoboxPropertyPriority(propertyKey, entityType string, configured map[string]int) int {
	if index, ok := configured[propertyKey]; ok {
		return index
	}
	if len(configured) > 0 {
		return len(configured) + 100
	}
	workOrder := map[string]int{
		"document_identifier": 0,
		"document_category":   1,
		"document_status":     2,
		"author":              3,
		"issued_by":           4,
		"release_date":        5,
		"updates":             6,
		"obsoletes":           7,
		"part_of":             8,
	}
	if entityType == "work" {
		if priority, ok := workOrder[propertyKey]; ok {
			return priority
		}
	}
	return 100
}

func infoboxVerificationSummary(groups []infoboxClaimGroup) string {
	unverified, disputed := 0, 0
	for _, group := range groups {
		for _, row := range group.rows {
			switch row.verificationStatus {
			case "disputed":
				disputed++
			case "unverified":
				unverified++
			}
		}
	}
	parts := make([]string, 0, 2)
	if disputed > 0 {
		parts = append(parts, fmt.Sprintf("%d 条事实存在争议", disputed))
	}
	if unverified > 0 {
		parts = append(parts, fmt.Sprintf("%d 条事实待核验", unverified))
	}
	return strings.Join(parts, " · ")
}

// InfoboxClaimIDs 返回信息框当前实际展示的 Claim，供 claim_usage 投影复用。
func InfoboxClaimIDs(
	ctx context.Context, q db.Querier, entityID uuid.UUID, displayConfig json.RawMessage,
) ([]uuid.UUID, error) {
	config, err := parseInfoboxConfig(displayConfig)
	if err != nil {
		return nil, err
	}
	rows, err := resolveInfoboxClaims(ctx, q, entityID, config)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.claimID)
	}
	return ids, nil
}
