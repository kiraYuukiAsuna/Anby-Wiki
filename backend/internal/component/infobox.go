package component

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/platform/db"
)

// RendererEntityClaimInfobox 是 M9-T03 内置信息框渲染器的白名单引用。
const RendererEntityClaimInfobox = "builtin.entity_claim_infobox"

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
	entityLabel, err := resolveEntityLabel(ctx, r.q, entityID, config.Language)
	if err != nil {
		return "", err
	}
	rows, err := resolveInfoboxClaims(ctx, r.q, entityID, config)
	if err != nil {
		return "", err
	}

	title := config.Title
	if title == "" {
		title = entityLabel
	}
	var out strings.Builder
	fmt.Fprintf(&out,
		`<aside class="entity-infobox" data-component-renderer="entity-claim-infobox" data-entity-id="%s">`,
		html.EscapeString(entityID.String()))
	fmt.Fprintf(&out, `<h2>%s</h2><dl>`, html.EscapeString(title))
	for _, row := range rows {
		fmt.Fprintf(&out,
			`<div data-claim-id="%s" data-verification-status="%s"><dt>%s</dt><dd>%s</dd></div>`,
			html.EscapeString(row.claimID.String()),
			html.EscapeString(row.verificationStatus),
			html.EscapeString(row.propertyName),
			html.EscapeString(row.value),
		)
	}
	out.WriteString("</dl></aside>")
	return out.String(), nil
}

type infoboxClaim struct {
	claimID            uuid.UUID
	propertyName       string
	value              string
	verificationStatus string
}

func parseInfoboxConfig(raw json.RawMessage) (infoboxConfig, error) {
	var config infoboxConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return config, fmt.Errorf("%w: display_config must be an object", ErrInvalidProps)
	}
	return config, nil
}

func resolveEntityLabel(
	ctx context.Context, q db.Querier, entityID uuid.UUID, language string,
) (string, error) {
	var label string
	err := q.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT el.label FROM entity_label el
			 WHERE el.entity_id = e.id AND el.is_primary
			   AND ($2 = '' OR el.language = $2)
			 ORDER BY (el.language = $2) DESC, el.language, el.label LIMIT 1),
			e.canonical_key)
		FROM entity e
		WHERE e.id = $1 AND e.status <> 'deleted'`, entityID, language).Scan(&label)
	if err != nil {
		return "", fmt.Errorf("component: resolve entity %s label: %w", entityID, err)
	}
	return label, nil
}

func resolveInfoboxClaims(
	ctx context.Context, q db.Querier, entityID uuid.UUID, config infoboxConfig,
) ([]infoboxClaim, error) {
	rows, err := q.Query(ctx, `
		SELECT c.id, p.name, c.value_type, c.value_json, c.target_entity_id,
		       c.verification_status,
		       COALESCE(
		         (SELECT el.label FROM entity_label el
		          WHERE el.entity_id = c.target_entity_id AND el.is_primary
		            AND ($3 = '' OR el.language = $3)
		          ORDER BY (el.language = $3) DESC, el.language, el.label LIMIT 1),
		         target.canonical_key)
		FROM claim c
		JOIN property p ON p.id = c.property_id
		LEFT JOIN entity target ON target.id = c.target_entity_id
		WHERE c.subject_entity_id = $1
		  AND c.status = 'published'
		  AND c.rank <> 'deprecated'
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
			targetLabel *string
		)
		if err := rows.Scan(
			&row.claimID, &row.propertyName, &valueType, &valueJSON, &targetID,
			&row.verificationStatus, &targetLabel,
		); err != nil {
			return nil, fmt.Errorf("component: scan infobox claim: %w", err)
		}
		row.value, err = infoboxValueText(valueType, valueJSON, targetID, targetLabel)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("component: iterate infobox claims: %w", err)
	}
	return result, nil
}

func infoboxValueText(
	valueType string, raw json.RawMessage, targetID *uuid.UUID, targetLabel *string,
) (string, error) {
	if valueType == "entity" {
		if targetLabel != nil {
			return *targetLabel, nil
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
