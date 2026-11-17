package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

func schemaType(raw any) string {
	schema, ok := raw.(map[string]any)
	if !ok {
		return "any"
	}
	if value, ok := schema["type"].(string); ok {
		return value
	}
	if value, ok := schema["type"]; ok {
		return compactSchemaValue(value)
	}
	if _, ok := schema["properties"]; ok {
		return "object"
	}
	if _, ok := schema["items"]; ok {
		return "array"
	}
	return "any"
}

func compactSchemaValue(value any) string {
	if text, ok := value.(string); ok {
		return strings.Join(strings.Fields(text), " ")
	}
	raw, _ := json.Marshal(value)
	return string(raw)
}

func schemaDetails(raw any) string {
	schema, ok := raw.(map[string]any)
	if !ok {
		return compactSchemaValue(raw)
	}
	details := []string{}
	if description, ok := schema["description"]; ok {
		details = append(details, compactSchemaValue(description))
	}
	for _, key := range []string{"$ref", "format", "enum", "const", "default", "example", "examples", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf", "minLength", "maxLength", "pattern", "minItems", "maxItems", "uniqueItems", "minProperties", "maxProperties", "additionalProperties", "readOnly", "writeOnly", "deprecated"} {
		if value, ok := schema[key]; ok {
			details = append(details, key+"="+compactSchemaValue(value))
		}
	}
	return strings.Join(details, "; ")
}

func writeSchemaFields(writer io.Writer, path string, raw any, required bool) {
	_, _ = fmt.Fprintf(writer, "%s\t%s\t%t\t%s\n", path, schemaType(raw), required, schemaDetails(raw))
	schema, ok := raw.(map[string]any)
	if !ok {
		return
	}
	requiredNames := map[string]bool{}
	if names, ok := schema["required"].([]any); ok {
		for _, name := range names {
			if text, ok := name.(string); ok {
				requiredNames[text] = true
			}
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		writeSchemaFields(writer, path+"."+name, properties[name], requiredNames[name])
	}
	if items, ok := schema["items"]; ok {
		writeSchemaFields(writer, path+"[]", items, false)
	}
	for _, keyword := range []string{"allOf", "oneOf", "anyOf"} {
		if branches, ok := schema[keyword].([]any); ok {
			for i, branch := range branches {
				writeSchemaFields(writer, fmt.Sprintf("%s (%s %d)", path, keyword, i+1), branch, false)
			}
		}
	}
}
