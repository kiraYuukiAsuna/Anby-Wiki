// Package clicontract exposes the OpenAPI contract to the agent CLI.
package clicontract

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

//go:embed openapi.yaml
var openAPIYAML []byte

var ErrOperationNotFound = errors.New("CLI contract: operation not found")

type Parameter struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
	Schema   any    `json:"schema"`
}

type RequestBody struct {
	Required     bool           `json:"required"`
	ContentTypes []string       `json:"content_types"`
	Schemas      map[string]any `json:"schemas"`
}

type Response struct {
	Status       string         `json:"status"`
	ContentTypes []string       `json:"content_types"`
	Schemas      map[string]any `json:"schemas"`
}

type Descriptor struct {
	ID          string       `json:"operation_id"`
	Method      string       `json:"method"`
	Path        string       `json:"path"`
	Summary     string       `json:"summary"`
	Tags        []string     `json:"tags"`
	CLIOnly     bool         `json:"cli_only"`
	Parameters  []Parameter  `json:"parameters"`
	RequestBody *RequestBody `json:"request_body"`
	Responses   []Response   `json:"responses"`
}

type compiledOperation struct {
	descriptor      Descriptor
	schemaDefs      any
	rawPathSchema   any
	rawQuerySchema  any
	rawHeaderSchema any
	rawBodySchemas  map[string]any
	rawResponses    map[string]map[string]any
	compileOnce     sync.Once
	compileErr      error
	pathValidator   *jsonschema.Schema
	queryValidator  *jsonschema.Schema
	headerValidator *jsonschema.Schema
	bodyValidators  map[string]*jsonschema.Schema
	responseSchemas map[string]map[string]*jsonschema.Schema
}

type Contract struct {
	operations map[string]*compiledOperation
	order      []string
}

type Call struct {
	Path        map[string]any
	Query       map[string]any
	Headers     map[string]any
	Body        any
	BodyPresent bool
	ContentType string
}

func Load() (*Contract, error) {
	var yamlDocument any
	if err := yaml.Unmarshal(openAPIYAML, &yamlDocument); err != nil {
		return nil, fmt.Errorf("CLI contract: parse OpenAPI: %w", err)
	}
	root, ok := stringMap(yamlDocument).(map[string]any)
	if !ok {
		return nil, errors.New("CLI contract: OpenAPI root is not an object")
	}
	paths, ok := root["paths"].(map[string]any)
	if !ok {
		return nil, errors.New("CLI contract: OpenAPI paths are missing")
	}
	components, _ := root["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	normalizedSchemas := normalizeSchema(schemas)

	result := &Contract{operations: map[string]*compiledOperation{}}
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]any)
		if !ok {
			continue
		}
		pathParameters := parameterList(root, pathItem["parameters"])
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			rawOperation, exists := pathItem[method]
			if !exists {
				continue
			}
			operationMap, ok := rawOperation.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("CLI contract: %s %s is not an object", method, path)
			}
			operation, err := compileOperation(
				root, normalizedSchemas, strings.ToUpper(method), path,
				pathParameters, operationMap,
			)
			if err != nil {
				return nil, err
			}
			if _, duplicate := result.operations[operation.descriptor.ID]; duplicate {
				return nil, fmt.Errorf(
					"CLI contract: duplicate operationId %q",
					operation.descriptor.ID,
				)
			}
			result.operations[operation.descriptor.ID] = operation
			result.order = append(result.order, operation.descriptor.ID)
		}
	}
	sort.Strings(result.order)
	if len(result.order) == 0 {
		return nil, errors.New("CLI contract: no operations found")
	}
	return result, nil
}

func (c *Contract) List() []Descriptor {
	result := make([]Descriptor, 0, len(c.order))
	for _, id := range c.order {
		result = append(result, c.operations[id].descriptor)
	}
	return result
}

func (c *Contract) Describe(id string) (Descriptor, error) {
	operation, ok := c.operations[strings.TrimSpace(id)]
	if !ok {
		return Descriptor{}, ErrOperationNotFound
	}
	return operation.descriptor, nil
}

func (c *Contract) ValidateCall(operationID string, call Call) error {
	operation, ok := c.operations[operationID]
	if !ok {
		return ErrOperationNotFound
	}
	if err := operation.ensureCompiled(); err != nil {
		return err
	}
	if err := validateValue(operation.pathValidator, objectOrEmpty(call.Path)); err != nil {
		return fmt.Errorf("path: %w", err)
	}
	if err := validateValue(operation.queryValidator, objectOrEmpty(call.Query)); err != nil {
		return fmt.Errorf("query: %w", err)
	}
	if err := validateValue(operation.headerValidator, objectOrEmpty(call.Headers)); err != nil {
		return fmt.Errorf("headers: %w", err)
	}
	if operation.descriptor.RequestBody == nil {
		if call.BodyPresent {
			return errors.New("body: operation does not accept a request body")
		}
		return nil
	}
	if operation.descriptor.RequestBody.Required && !call.BodyPresent {
		return errors.New("body: required request body is missing")
	}
	if !call.BodyPresent {
		return nil
	}
	contentType := baseContentType(call.ContentType)
	validator, ok := operation.bodyValidators[contentType]
	if !ok {
		return fmt.Errorf("body: unsupported content type %q", contentType)
	}
	if err := validateValue(validator, call.Body); err != nil {
		return fmt.Errorf("body: %w", err)
	}
	return nil
}

func (c *Contract) ValidateResponse(
	operationID string,
	status int,
	contentType string,
	value any,
) error {
	operation, ok := c.operations[operationID]
	if !ok {
		return ErrOperationNotFound
	}
	if err := operation.ensureCompiled(); err != nil {
		return err
	}
	statusKey := fmt.Sprintf("%d", status)
	byContent, ok := operation.responseSchemas[statusKey]
	if !ok {
		byContent = operation.responseSchemas["default"]
	}
	if len(byContent) == 0 {
		return nil
	}
	validator, ok := byContent[baseContentType(contentType)]
	if !ok {
		return nil
	}
	if err := validateValue(validator, value); err != nil {
		return fmt.Errorf("response status %d: %w", status, err)
	}
	return nil
}

func (operation *compiledOperation) ensureCompiled() error {
	operation.compileOnce.Do(func() {
		id := operation.descriptor.ID
		var err error
		operation.pathValidator, err = compileJSONSchema(
			operation.schemaDefs, operation.rawPathSchema, id+"-path-parameters",
		)
		if err != nil {
			operation.compileErr = err
			return
		}
		operation.queryValidator, err = compileJSONSchema(
			operation.schemaDefs, operation.rawQuerySchema, id+"-query-parameters",
		)
		if err != nil {
			operation.compileErr = err
			return
		}
		operation.headerValidator, err = compileJSONSchema(
			operation.schemaDefs, operation.rawHeaderSchema, id+"-header-parameters",
		)
		if err != nil {
			operation.compileErr = err
			return
		}
		for contentType, raw := range operation.rawBodySchemas {
			operation.bodyValidators[contentType], err = compileJSONSchema(
				operation.schemaDefs, raw, id+"-body-"+contentType,
			)
			if err != nil {
				operation.compileErr = err
				return
			}
		}
		for status, rawSchemas := range operation.rawResponses {
			operation.responseSchemas[status] = map[string]*jsonschema.Schema{}
			for contentType, raw := range rawSchemas {
				operation.responseSchemas[status][contentType], err = compileJSONSchema(
					operation.schemaDefs, raw,
					id+"-response-"+status+"-"+contentType,
				)
				if err != nil {
					operation.compileErr = err
					return
				}
			}
		}
	})
	return operation.compileErr
}

func compileOperation(
	root map[string]any,
	schemas any,
	method, path string,
	pathParameters []Parameter,
	raw map[string]any,
) (*compiledOperation, error) {
	id, _ := raw["operationId"].(string)
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("CLI contract: %s %s lacks operationId", method, path)
	}
	parameters := append([]Parameter{}, pathParameters...)
	parameters = append(parameters, parameterList(root, raw["parameters"])...)
	parameters = uniqueParameters(parameters)

	operation := &compiledOperation{
		descriptor: Descriptor{
			ID: id, Method: method, Path: path,
			Summary: stringValue(raw["summary"]),
			Tags:    stringSlice(raw["tags"]),
			CLIOnly: boolValue(raw["x-anby-cli-only"]),
		},
		schemaDefs:      schemas,
		rawBodySchemas:  map[string]any{},
		rawResponses:    map[string]map[string]any{},
		bodyValidators:  map[string]*jsonschema.Schema{},
		responseSchemas: map[string]map[string]*jsonschema.Schema{},
	}
	operation.descriptor.Parameters = parameters

	operation.rawPathSchema = parameterSchema(parameters, "path")
	operation.rawQuerySchema = parameterSchema(parameters, "query")
	operation.rawHeaderSchema = parameterSchema(parameters, "header")

	if rawBody, exists := raw["requestBody"]; exists {
		body := resolveObjectRef(root, rawBody)
		content, _ := body["content"].(map[string]any)
		descriptor := &RequestBody{
			Required: boolValue(body["required"]),
			Schemas:  map[string]any{},
		}
		for contentType, rawMedia := range content {
			media, _ := rawMedia.(map[string]any)
			schema := normalizeSchema(media["schema"])
			descriptor.ContentTypes = append(descriptor.ContentTypes, contentType)
			descriptor.Schemas[contentType] = resolveSchemaForDisplay(root, schema, map[string]bool{})
			operation.rawBodySchemas[baseContentType(contentType)] = schema
		}
		sort.Strings(descriptor.ContentTypes)
		operation.descriptor.RequestBody = descriptor
	}

	rawResponses, _ := raw["responses"].(map[string]any)
	statuses := make([]string, 0, len(rawResponses))
	for status := range rawResponses {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	for _, status := range statuses {
		responseMap := resolveObjectRef(root, rawResponses[status])
		content, _ := responseMap["content"].(map[string]any)
		response := Response{Status: status, Schemas: map[string]any{}}
		operation.rawResponses[status] = map[string]any{}
		for contentType, rawMedia := range content {
			media, _ := rawMedia.(map[string]any)
			schema := normalizeSchema(media["schema"])
			response.ContentTypes = append(response.ContentTypes, contentType)
			response.Schemas[contentType] = resolveSchemaForDisplay(root, schema, map[string]bool{})
			operation.rawResponses[status][baseContentType(contentType)] = schema
		}
		sort.Strings(response.ContentTypes)
		operation.descriptor.Responses = append(operation.descriptor.Responses, response)
	}
	return operation, nil
}

func parameterSchema(
	parameters []Parameter,
	location string,
) any {
	properties := map[string]any{}
	required := []string{}
	for _, parameter := range parameters {
		if parameter.In != location {
			continue
		}
		properties[parameter.Name] = normalizeSchema(parameter.Schema)
		if parameter.Required {
			required = append(required, parameter.Name)
		}
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func compileJSONSchema(
	schemas any,
	raw any,
	name string,
) (*jsonschema.Schema, error) {
	document := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$defs":   rewriteSchemaRefs(normalizeSchema(schemas)),
		"allOf":   []any{rewriteSchemaRefs(normalizeSchema(raw))},
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("CLI contract: encode schema %s: %w", name, err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("CLI contract: decode schema %s: %w", name, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	resource := "https://anby.wiki/cli-schema/" + sanitizeResourceName(name)
	if err := compiler.AddResource(resource, value); err != nil {
		return nil, fmt.Errorf("CLI contract: register schema %s: %w", name, err)
	}
	schema, err := compiler.Compile(resource)
	if err != nil {
		return nil, fmt.Errorf("CLI contract: compile schema %s: %w", name, err)
	}
	return schema, nil
}

func parameterList(root map[string]any, raw any) []Parameter {
	values, _ := raw.([]any)
	result := make([]Parameter, 0, len(values))
	for _, item := range values {
		value := resolveObjectRef(root, item)
		name, _ := value["name"].(string)
		location, _ := value["in"].(string)
		if name == "" || location == "" {
			continue
		}
		result = append(result, Parameter{
			Name: name, In: location,
			Required: boolValue(value["required"]),
			Schema:   normalizeSchema(value["schema"]),
		})
	}
	return result
}

func uniqueParameters(values []Parameter) []Parameter {
	result := make([]Parameter, 0, len(values))
	index := map[string]int{}
	for _, value := range values {
		key := value.In + "\x00" + strings.ToLower(value.Name)
		if existing, ok := index[key]; ok {
			result[existing] = value
			continue
		}
		index[key] = len(result)
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].In == result[j].In {
			return result[i].Name < result[j].Name
		}
		return result[i].In < result[j].In
	})
	return result
}

func validateValue(schema *jsonschema.Schema, value any) error {
	if schema == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	return schema.Validate(instance)
}

func resolveObjectRef(root map[string]any, raw any) map[string]any {
	value, _ := raw.(map[string]any)
	if reference, ok := value["$ref"].(string); ok {
		if resolved, ok := resolveJSONPointer(root, reference).(map[string]any); ok {
			return resolved
		}
	}
	return value
}

func resolveSchemaForDisplay(
	root map[string]any,
	raw any,
	stack map[string]bool,
) any {
	switch value := raw.(type) {
	case map[string]any:
		if reference, ok := value["$ref"].(string); ok &&
			strings.HasPrefix(reference, "#/components/schemas/") {
			if stack[reference] {
				return map[string]any{"$ref": reference}
			}
			stack[reference] = true
			resolved := resolveSchemaForDisplay(
				root, normalizeSchema(resolveJSONPointer(root, reference)), stack,
			)
			delete(stack, reference)
			return resolved
		}
		result := make(map[string]any, len(value))
		for key, child := range value {
			result[key] = resolveSchemaForDisplay(root, child, stack)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index := range value {
			result[index] = resolveSchemaForDisplay(root, value[index], stack)
		}
		return result
	default:
		return value
	}
}

func resolveJSONPointer(root map[string]any, reference string) any {
	if !strings.HasPrefix(reference, "#/") {
		return nil
	}
	var current any = root
	for _, part := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[part]
	}
	return current
}

func normalizeSchema(raw any) any {
	switch value := raw.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, child := range value {
			if key == "nullable" {
				continue
			}
			result[key] = normalizeSchema(child)
		}
		if boolValue(value["nullable"]) {
			if currentType, ok := result["type"].(string); ok {
				result["type"] = []any{currentType, "null"}
			} else {
				copy := make(map[string]any, len(result))
				for key, child := range result {
					copy[key] = child
				}
				result = map[string]any{
					"anyOf": []any{copy, map[string]any{"type": "null"}},
				}
			}
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index := range value {
			result[index] = normalizeSchema(value[index])
		}
		return result
	default:
		return value
	}
}

func rewriteSchemaRefs(raw any) any {
	switch value := raw.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, child := range value {
			if key == "$ref" {
				if reference, ok := child.(string); ok {
					result[key] = strings.Replace(
						reference, "#/components/schemas/", "#/$defs/", 1,
					)
					continue
				}
			}
			result[key] = rewriteSchemaRefs(child)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index := range value {
			result[index] = rewriteSchemaRefs(value[index])
		}
		return result
	default:
		return value
	}
}

func stringMap(raw any) any {
	switch value := raw.(type) {
	case map[any]any:
		result := make(map[string]any, len(value))
		for key, child := range value {
			text, ok := key.(string)
			if !ok {
				continue
			}
			result[text] = stringMap(child)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, child := range value {
			result[key] = stringMap(child)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index := range value {
			result[index] = stringMap(value[index])
		}
		return result
	default:
		return value
	}
}

func objectOrEmpty(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func baseContentType(value string) string {
	if before, _, found := strings.Cut(value, ";"); found {
		value = before
	}
	return strings.TrimSpace(strings.ToLower(value))
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func stringSlice(value any) []string {
	raw, _ := value.([]any)
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func sanitizeResourceName(value string) string {
	replacer := strings.NewReplacer(
		" ", "-", "/", "-", "{", "", "}", "", ";", "-", ":", "-",
	)
	return replacer.Replace(value)
}
