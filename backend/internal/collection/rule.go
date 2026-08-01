package collection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func ParseRule(raw json.RawMessage) (Rule, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var rule Rule
	if err := decoder.Decode(&rule); err != nil {
		return Rule{}, fmt.Errorf("%w: %v", ErrInvalidRule, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF || rule.Version != RuleVersion {
		return Rule{}, ErrInvalidRule
	}
	rule.Kind = strings.TrimSpace(rule.Kind)
	rule.EntityType = strings.TrimSpace(rule.EntityType)
	rule.Property = strings.TrimSpace(rule.Property)
	switch rule.Kind {
	case "entity_type":
		if rule.EntityType == "" || rule.Property != "" {
			return Rule{}, ErrInvalidRule
		}
	case "claim_exists":
		if rule.Property == "" || rule.EntityType != "" {
			return Rule{}, ErrInvalidRule
		}
	default:
		return Rule{}, ErrInvalidRule
	}
	return rule, nil
}

func ParseDynamicQuery(raw json.RawMessage) (DynamicQuery, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var query DynamicQuery
	if err := decoder.Decode(&query); err != nil {
		return query, fmt.Errorf("%w: %v", ErrInvalidRule, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF ||
		query.Version != DynamicQueryVersion {
		return query, ErrInvalidRule
	}
	query.MemberType = strings.TrimSpace(query.MemberType)
	query.Text = strings.TrimSpace(query.Text)
	query.Namespace = strings.TrimSpace(query.Namespace)
	query.EntityType = strings.TrimSpace(query.EntityType)
	query.Property = strings.TrimSpace(query.Property)
	if len([]rune(query.Text)) > 200 {
		return query, ErrInvalidRule
	}
	switch query.MemberType {
	case MemberPage:
		if query.EntityType != "" || query.Property != "" {
			return query, ErrInvalidRule
		}
	case MemberEntity:
		if query.Namespace != "" {
			return query, ErrInvalidRule
		}
	default:
		return query, ErrInvalidRule
	}
	return query, nil
}
