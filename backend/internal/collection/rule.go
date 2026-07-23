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
