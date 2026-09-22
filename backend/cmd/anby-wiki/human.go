package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/anby/wiki/backend/internal/clicontract"
	"github.com/anby/wiki/backend/internal/wikicli"
)

type outputMode int

const (
	outputJSON outputMode = iota
	outputHuman
)

type cliInvocation struct {
	reader     io.Reader
	closeInput func()
	input      *wikicli.Input
	output     outputMode
	helpText   string
}

type humanOptions struct {
	baseURL    string
	configPath string
	timeout    int
	jsonOutput bool
}

type callOptions struct {
	humanOptions
	path      map[string]any
	query     map[string]any
	headers   map[string]any
	fields    map[string]any
	files     map[string]string
	body      json.RawMessage
	bodySet   bool
	bodyFile  string
	operation clicontract.Descriptor
}

type argumentError struct {
	message string
}

func (e *argumentError) Error() string {
	return e.message
}

func buildInvocation(arguments []string, stdin io.Reader) (cliInvocation, error) {
	if len(arguments) == 0 {
		return cliInvocation{reader: stdin, output: outputJSON}, nil
	}
	if arguments[0] == "--input" {
		if len(arguments) != 2 {
			return cliInvocation{output: outputJSON}, &argumentError{
				message: "usage: anby-wiki [--input request.json] | anby-wiki <command> [flags]",
			}
		}
		file, err := os.Open(arguments[1])
		if err != nil {
			return cliInvocation{output: outputJSON}, err
		}
		return cliInvocation{
			reader: file, closeInput: func() { _ = file.Close() },
			output: outputJSON,
		}, nil
	}

	input, output, helpText, err := parseHumanArguments(arguments)
	invocation := cliInvocation{input: &input, output: output, helpText: helpText}
	if helpText != "" {
		invocation.input = nil
	}
	return invocation, err
}

func parseHumanArguments(arguments []string) (wikicli.Input, outputMode, string, error) {
	output := outputHuman
	if len(arguments) == 0 {
		return wikicli.Input{}, output, humanHelp(), nil
	}
	arguments, output = consumeLeadingJSONFlag(arguments, output)
	if len(arguments) == 0 {
		return wikicli.Input{}, output, humanHelp(), nil
	}
	if isHelpToken(arguments[0]) {
		return wikicli.Input{}, output, humanHelp(), nil
	}

	command := arguments[0]
	rest := arguments[1:]
	switch command {
	case "help":
		return wikicli.Input{}, output, humanHelp(), nil
	case "version":
		options, positionals, err := parseCommonOptions(rest, output)
		if err != nil {
			return wikicli.Input{}, modeFromOptions(output, options), "", err
		}
		if len(positionals) > 0 {
			return wikicli.Input{}, modeFromOptions(output, options), "",
				unexpectedArgument(positionals[0])
		}
		return applyCommonOptions(wikicli.Input{Action: "version"}, options),
			modeFromOptions(output, options), "", nil
	case "auth":
		return parseAuthCommand(rest, output)
	case "config":
		return parseConfigCommand(rest, output)
	case "operations":
		return parseOperationsCommand(rest, output)
	case "describe":
		return parseDescribeCommand(rest, output)
	case "collaboration":
		return parseCollaborationCommand(rest, output)
	case "call":
		return parseCallCommand(rest, output)
	default:
		return parseDirectOperationCommand(command, rest, output)
	}
}

func parseAuthCommand(arguments []string, output outputMode) (
	wikicli.Input,
	outputMode,
	string,
	error,
) {
	if len(arguments) == 0 || isHelpToken(arguments[0]) {
		return wikicli.Input{}, output, authHelp(), nil
	}
	if containsHelpToken(arguments[1:]) {
		return wikicli.Input{}, output, authHelp(), nil
	}
	switch arguments[0] {
	case "exchange":
		options, positionals, persist, code, err := parseAuthExchangeOptions(arguments[1:], output)
		mode := modeFromOptions(output, options)
		if err != nil {
			return wikicli.Input{}, mode, "", err
		}
		if code == "" && len(positionals) > 0 {
			code = positionals[0]
			positionals = positionals[1:]
		}
		if len(positionals) > 0 {
			return wikicli.Input{}, mode, "", unexpectedArgument(positionals[0])
		}
		input := applyCommonOptions(wikicli.Input{
			Action: "auth.exchange", Code: strings.TrimSpace(code),
		}, options)
		input.Persist = persist
		return input, mode, "", nil
	case "status":
		options, positionals, err := parseCommonOptions(arguments[1:], output)
		mode := modeFromOptions(output, options)
		if err != nil {
			return wikicli.Input{}, mode, "", err
		}
		if len(positionals) > 0 {
			return wikicli.Input{}, mode, "", unexpectedArgument(positionals[0])
		}
		return applyCommonOptions(wikicli.Input{Action: "auth.status"}, options), mode, "", nil
	case "logout":
		options, positionals, err := parseCommonOptions(arguments[1:], output)
		mode := modeFromOptions(output, options)
		if err != nil {
			return wikicli.Input{}, mode, "", err
		}
		if len(positionals) > 0 {
			return wikicli.Input{}, mode, "", unexpectedArgument(positionals[0])
		}
		return applyCommonOptions(wikicli.Input{Action: "auth.logout"}, options), mode, "", nil
	default:
		return wikicli.Input{}, output, "", fmt.Errorf(
			"unknown auth command %q; use auth exchange, auth status, or auth logout",
			arguments[0],
		)
	}
}

func parseAuthExchangeOptions(arguments []string, output outputMode) (
	humanOptions,
	[]string,
	*bool,
	string,
	error,
) {
	options := humanOptions{jsonOutput: output == outputJSON}
	positionals := []string{}
	var persist *bool
	code := ""
	for index := 0; index < len(arguments); {
		raw := arguments[index]
		if raw == "--" {
			positionals = append(positionals, arguments[index+1:]...)
			break
		}
		name, value, hasValue, ok := splitLongFlag(raw)
		if !ok {
			positionals = append(positionals, raw)
			index++
			continue
		}
		switch name {
		case "code":
			next, err := optionValue(arguments, index, value, hasValue, name)
			if err != nil {
				return options, nil, persist, "", err
			}
			code = next.value
			index = next.index
		case "persist":
			parsed, err := boolOption(index, value, hasValue, name, true)
			if err != nil {
				return options, nil, persist, "", err
			}
			persist = &parsed.value
			index = parsed.index
		case "no-persist":
			value := false
			persist = &value
			index++
		default:
			consumed, err := applyCommonOption(&options, arguments, index, value, hasValue, name)
			if err != nil {
				return options, nil, persist, "", err
			}
			if consumed == index {
				return options, nil, persist, "", unknownFlag(name)
			}
			index = consumed
		}
	}
	return options, positionals, persist, code, nil
}

func parseConfigCommand(arguments []string, output outputMode) (
	wikicli.Input,
	outputMode,
	string,
	error,
) {
	if len(arguments) == 0 || isHelpToken(arguments[0]) {
		return wikicli.Input{}, output, configHelp(), nil
	}
	if containsHelpToken(arguments[1:]) {
		return wikicli.Input{}, output, configHelp(), nil
	}
	if arguments[0] != "show" {
		return wikicli.Input{}, output, "", fmt.Errorf(
			"unknown config command %q; use config show",
			arguments[0],
		)
	}
	options, positionals, err := parseCommonOptions(arguments[1:], output)
	mode := modeFromOptions(output, options)
	if err != nil {
		return wikicli.Input{}, mode, "", err
	}
	if len(positionals) > 0 {
		return wikicli.Input{}, mode, "", unexpectedArgument(positionals[0])
	}
	return applyCommonOptions(wikicli.Input{Action: "config.show"}, options), mode, "", nil
}

func parseOperationsCommand(arguments []string, output outputMode) (
	wikicli.Input,
	outputMode,
	string,
	error,
) {
	if len(arguments) == 0 || isHelpToken(arguments[0]) {
		return wikicli.Input{}, output, operationsHelp(), nil
	}
	if containsHelpToken(arguments[1:]) {
		if arguments[0] == "describe" {
			return wikicli.Input{}, output, describeHelp(), nil
		}
		return wikicli.Input{}, output, operationsHelp(), nil
	}
	switch arguments[0] {
	case "list":
		options, positionals, tag, search, err := parseListOptions(arguments[1:], output)
		mode := modeFromOptions(output, options)
		if err != nil {
			return wikicli.Input{}, mode, "", err
		}
		if len(positionals) > 0 {
			return wikicli.Input{}, mode, "", unexpectedArgument(positionals[0])
		}
		return applyCommonOptions(wikicli.Input{
			Action: "operations.list", Tag: tag, Search: search,
		}, options), mode, "", nil
	case "describe":
		return parseDescribeCommand(arguments[1:], output)
	default:
		return wikicli.Input{}, output, "", fmt.Errorf(
			"unknown operations command %q; use operations list or operations describe",
			arguments[0],
		)
	}
}

func parseListOptions(arguments []string, output outputMode) (
	humanOptions,
	[]string,
	string,
	string,
	error,
) {
	options := humanOptions{jsonOutput: output == outputJSON}
	positionals := []string{}
	tag := ""
	search := ""
	for index := 0; index < len(arguments); {
		raw := arguments[index]
		if raw == "--" {
			positionals = append(positionals, arguments[index+1:]...)
			break
		}
		name, value, hasValue, ok := splitLongFlag(raw)
		if !ok {
			positionals = append(positionals, raw)
			index++
			continue
		}
		switch name {
		case "tag":
			next, err := optionValue(arguments, index, value, hasValue, name)
			if err != nil {
				return options, nil, "", "", err
			}
			tag = next.value
			index = next.index
		case "search":
			next, err := optionValue(arguments, index, value, hasValue, name)
			if err != nil {
				return options, nil, "", "", err
			}
			search = next.value
			index = next.index
		default:
			consumed, err := applyCommonOption(&options, arguments, index, value, hasValue, name)
			if err != nil {
				return options, nil, "", "", err
			}
			if consumed == index {
				return options, nil, "", "", unknownFlag(name)
			}
			index = consumed
		}
	}
	return options, positionals, tag, search, nil
}

func parseDescribeCommand(arguments []string, output outputMode) (
	wikicli.Input,
	outputMode,
	string,
	error,
) {
	if len(arguments) == 0 || isHelpToken(arguments[0]) {
		return wikicli.Input{}, output, describeHelp(), nil
	}
	if containsHelpToken(arguments[1:]) {
		return wikicli.Input{}, output, describeHelp(), nil
	}
	options, positionals, err := parseCommonOptions(arguments[1:], output)
	mode := modeFromOptions(output, options)
	if err != nil {
		return wikicli.Input{}, mode, "", err
	}
	if len(positionals) > 0 {
		return wikicli.Input{}, mode, "", unexpectedArgument(positionals[0])
	}
	operationID, err := operationIDFromCommand(arguments[0])
	if err != nil {
		return wikicli.Input{}, mode, "", err
	}
	return applyCommonOptions(wikicli.Input{
		Action: "operation.describe", OperationID: operationID,
	}, options), mode, "", nil
}

func parseCollaborationCommand(arguments []string, output outputMode) (
	wikicli.Input,
	outputMode,
	string,
	error,
) {
	if len(arguments) == 0 || isHelpToken(arguments[0]) {
		return wikicli.Input{}, output, collaborationHelp(), nil
	}
	if containsHelpToken(arguments[1:]) {
		return wikicli.Input{}, output, collaborationHelp(), nil
	}
	if arguments[0] != "run" {
		return wikicli.Input{}, output, "", fmt.Errorf(
			"unknown collaboration command %q; use collaboration run",
			arguments[0],
		)
	}
	options := humanOptions{jsonOutput: output == outputJSON}
	positionals := []string{}
	pageID := ""
	clientID := ""
	lastSequence := int64(0)
	messages := []wikicli.CollaborationMessage{}
	for index := 1; index < len(arguments); {
		raw := arguments[index]
		if raw == "--" {
			positionals = append(positionals, arguments[index+1:]...)
			break
		}
		name, value, hasValue, ok := splitLongFlag(raw)
		if !ok {
			positionals = append(positionals, raw)
			index++
			continue
		}
		switch name {
		case "page-id":
			next, err := optionValue(arguments, index, value, hasValue, name)
			if err != nil {
				return wikicli.Input{}, modeFromOptions(output, options), "", err
			}
			pageID = next.value
			index = next.index
		case "client-id":
			next, err := optionValue(arguments, index, value, hasValue, name)
			if err != nil {
				return wikicli.Input{}, modeFromOptions(output, options), "", err
			}
			clientID = next.value
			index = next.index
		case "last-sequence":
			next, err := optionValue(arguments, index, value, hasValue, name)
			if err != nil {
				return wikicli.Input{}, modeFromOptions(output, options), "", err
			}
			lastSequence, err = strconv.ParseInt(next.value, 10, 64)
			if err != nil {
				return wikicli.Input{}, modeFromOptions(output, options), "",
					fmt.Errorf("--last-sequence must be an integer")
			}
			index = next.index
		case "message":
			next, err := optionValue(arguments, index, value, hasValue, name)
			if err != nil {
				return wikicli.Input{}, modeFromOptions(output, options), "", err
			}
			message, err := parseCollaborationMessage(next.value)
			if err != nil {
				return wikicli.Input{}, modeFromOptions(output, options), "", err
			}
			messages = append(messages, message)
			index = next.index
		case "messages":
			next, err := optionValue(arguments, index, value, hasValue, name)
			if err != nil {
				return wikicli.Input{}, modeFromOptions(output, options), "", err
			}
			values, err := parseCollaborationMessages(next.value)
			if err != nil {
				return wikicli.Input{}, modeFromOptions(output, options), "", err
			}
			messages = append(messages, values...)
			index = next.index
		default:
			consumed, err := applyCommonOption(&options, arguments, index, value, hasValue, name)
			if err != nil {
				return wikicli.Input{}, modeFromOptions(output, options), "", err
			}
			if consumed == index {
				return wikicli.Input{}, modeFromOptions(output, options), "", unknownFlag(name)
			}
			index = consumed
		}
	}
	mode := modeFromOptions(output, options)
	if len(positionals) > 0 {
		return wikicli.Input{}, mode, "", unexpectedArgument(positionals[0])
	}
	input := applyCommonOptions(wikicli.Input{
		Action:       "collaboration.run",
		PageID:       pageID,
		ClientID:     clientID,
		LastSequence: lastSequence,
		Messages:     messages,
	}, options)
	return input, mode, "", nil
}

func parseCallCommand(arguments []string, output outputMode) (
	wikicli.Input,
	outputMode,
	string,
	error,
) {
	if len(arguments) == 0 || isHelpToken(arguments[0]) {
		return wikicli.Input{}, output, callHelp(), nil
	}
	if containsHelpToken(arguments[1:]) {
		return wikicli.Input{}, output, callHelp(), nil
	}
	return parseOperationCall(arguments[0], arguments[1:], output)
}

func parseDirectOperationCommand(command string, arguments []string, output outputMode) (
	wikicli.Input,
	outputMode,
	string,
	error,
) {
	return parseOperationCall(command, arguments, output)
}

func parseOperationCall(command string, arguments []string, output outputMode) (
	wikicli.Input,
	outputMode,
	string,
	error,
) {
	if containsHelpToken(arguments) {
		return wikicli.Input{}, output, callHelp(), nil
	}
	operationID, err := operationIDFromCommand(command)
	if err != nil {
		return wikicli.Input{}, output, "", err
	}
	contract, err := clicontract.Load()
	if err != nil {
		return wikicli.Input{}, output, "", err
	}
	operation, err := contract.Describe(operationID)
	if err != nil {
		return wikicli.Input{}, output, "", err
	}
	options := callOptions{
		humanOptions: humanOptions{jsonOutput: output == outputJSON},
		path:         map[string]any{},
		query:        map[string]any{},
		headers:      map[string]any{},
		fields:       map[string]any{},
		files:        map[string]string{},
		operation:    operation,
	}
	positionals := []string{}
	for index := 0; index < len(arguments); {
		raw := arguments[index]
		if raw == "--" {
			positionals = append(positionals, arguments[index+1:]...)
			break
		}
		name, value, hasValue, ok := splitLongFlag(raw)
		if !ok {
			positionals = append(positionals, raw)
			index++
			continue
		}
		consumed, optionErr := applyCallOption(&options, arguments, index, value, hasValue, name)
		if optionErr != nil {
			return wikicli.Input{}, modeFromOptions(output, options.humanOptions), "", optionErr
		}
		index = consumed
	}
	mode := modeFromOptions(output, options.humanOptions)
	if len(positionals) > 0 {
		return wikicli.Input{}, mode, "", unexpectedArgument(positionals[0])
	}
	body, err := buildBody(options.body, options.bodySet, options.bodyFile, options.fields)
	if err != nil {
		return wikicli.Input{}, mode, "", err
	}
	input := applyCommonOptions(wikicli.Input{
		Action:      "operation.call",
		OperationID: operationID,
		Path:        nilIfEmpty(options.path),
		Query:       nilIfEmpty(options.query),
		Headers:     nilIfEmpty(options.headers),
		Body:        body,
		Files:       nilIfEmptyString(options.files),
	}, options.humanOptions)
	return input, mode, "", nil
}

func parseCommonOptions(arguments []string, output outputMode) (
	humanOptions,
	[]string,
	error,
) {
	options := humanOptions{jsonOutput: output == outputJSON}
	positionals := []string{}
	for index := 0; index < len(arguments); {
		raw := arguments[index]
		if raw == "--" {
			positionals = append(positionals, arguments[index+1:]...)
			break
		}
		name, value, hasValue, ok := splitLongFlag(raw)
		if !ok {
			positionals = append(positionals, raw)
			index++
			continue
		}
		consumed, err := applyCommonOption(&options, arguments, index, value, hasValue, name)
		if err != nil {
			return options, nil, err
		}
		if consumed == index {
			return options, nil, unknownFlag(name)
		}
		index = consumed
	}
	return options, positionals, nil
}

func applyCallOption(
	options *callOptions,
	arguments []string,
	index int,
	value string,
	hasValue bool,
	name string,
) (int, error) {
	if consumed, err := applyCommonOption(
		&options.humanOptions, arguments, index, value, hasValue, name,
	); err != nil {
		return 0, err
	} else if consumed != index {
		return consumed, nil
	}

	switch name {
	case "path":
		next, err := optionValue(arguments, index, value, hasValue, name)
		if err != nil {
			return 0, err
		}
		key, parsed, err := parseKeyValue(next.value)
		if err != nil {
			return 0, err
		}
		addValue(options.path, key, parsed)
		return next.index, nil
	case "query":
		next, err := optionValue(arguments, index, value, hasValue, name)
		if err != nil {
			return 0, err
		}
		key, parsed, err := parseKeyValue(next.value)
		if err != nil {
			return 0, err
		}
		addValue(options.query, key, parsed)
		return next.index, nil
	case "header":
		next, err := optionValue(arguments, index, value, hasValue, name)
		if err != nil {
			return 0, err
		}
		key, parsed, err := parseKeyValue(next.value)
		if err != nil {
			return 0, err
		}
		addValue(options.headers, key, parsed)
		return next.index, nil
	case "param":
		next, err := optionValue(arguments, index, value, hasValue, name)
		if err != nil {
			return 0, err
		}
		key, parsed, err := parseKeyValue(next.value)
		if err != nil {
			return 0, err
		}
		if err := addOperationParameter(options, key, parsed); err != nil {
			return 0, err
		}
		return next.index, nil
	case "field":
		next, err := optionValue(arguments, index, value, hasValue, name)
		if err != nil {
			return 0, err
		}
		key, parsed, err := parseKeyValue(next.value)
		if err != nil {
			return 0, err
		}
		addValue(options.fields, key, parsed)
		return next.index, nil
	case "file":
		next, err := optionValue(arguments, index, value, hasValue, name)
		if err != nil {
			return 0, err
		}
		key, path, err := parseStringKeyValue(next.value)
		if err != nil {
			return 0, err
		}
		options.files[key] = path
		return next.index, nil
	case "body":
		next, err := optionValue(arguments, index, value, hasValue, name)
		if err != nil {
			return 0, err
		}
		body, err := readJSONArgument(next.value)
		if err != nil {
			return 0, fmt.Errorf("body: %w", err)
		}
		options.body = body
		options.bodySet = true
		return next.index, nil
	case "body-file":
		next, err := optionValue(arguments, index, value, hasValue, name)
		if err != nil {
			return 0, err
		}
		options.bodyFile = next.value
		return next.index, nil
	default:
		next, err := optionValue(arguments, index, value, hasValue, name)
		if err != nil && options.operation.RequestBody == nil {
			return 0, unknownFlag(name)
		}
		parameter, ok := findOperationParameter(options.operation, name)
		if !ok {
			if options.operation.RequestBody == nil {
				return 0, unknownFlag(name)
			}
			if err != nil {
				return 0, err
			}
			addValue(options.fields, normalizeFlagName(name), parseCLIValue(next.value))
			return next.index, nil
		}
		addParameterValue(options, parameter, parseCLIValue(next.value))
		return next.index, nil
	}
}

func applyCommonOption(
	options *humanOptions,
	arguments []string,
	index int,
	value string,
	hasValue bool,
	name string,
) (int, error) {
	switch name {
	case "json":
		parsed, err := boolOption(index, value, hasValue, name, true)
		if err != nil {
			return 0, err
		}
		options.jsonOutput = parsed.value
		return parsed.index, nil
	case "base-url":
		next, err := optionValue(arguments, index, value, hasValue, name)
		if err != nil {
			return 0, err
		}
		options.baseURL = next.value
		return next.index, nil
	case "config":
		next, err := optionValue(arguments, index, value, hasValue, name)
		if err != nil {
			return 0, err
		}
		options.configPath = next.value
		return next.index, nil
	case "timeout":
		next, err := optionValue(arguments, index, value, hasValue, name)
		if err != nil {
			return 0, err
		}
		timeout, err := strconv.Atoi(next.value)
		if err != nil {
			return 0, fmt.Errorf("timeout must be an integer")
		}
		options.timeout = timeout
		return next.index, nil
	default:
		return index, nil
	}
}

func applyCommonOptions(input wikicli.Input, options humanOptions) wikicli.Input {
	input.BaseURL = options.baseURL
	input.ConfigPath = options.configPath
	input.TimeoutSeconds = options.timeout
	return input
}

func buildBody(
	body json.RawMessage,
	bodySet bool,
	bodyFile string,
	fields map[string]any,
) (json.RawMessage, error) {
	if bodySet && strings.TrimSpace(bodyFile) != "" {
		return nil, errors.New("use either --body or --body-file, not both")
	}
	if strings.TrimSpace(bodyFile) != "" {
		raw, err := os.ReadFile(bodyFile)
		if err != nil {
			return nil, fmt.Errorf("read body file: %w", err)
		}
		body = raw
		bodySet = true
	}
	if len(fields) == 0 {
		return body, nil
	}
	object := map[string]any{}
	if bodySet {
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		if err := decoder.Decode(&object); err != nil {
			return nil, fmt.Errorf("decode body object: %w", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return nil, errors.New("decode body object: expected one JSON object")
		}
	}
	for key, value := range fields {
		object[key] = value
	}
	raw, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func operationIDFromCommand(command string) (string, error) {
	contract, err := clicontract.Load()
	if err != nil {
		return "", err
	}
	if _, err := contract.Describe(command); err == nil {
		return command, nil
	}
	for _, operation := range contract.List() {
		if commandName(operation.ID) == command {
			return operation.ID, nil
		}
	}
	return "", fmt.Errorf("unknown command or operation %q; use operations list", command)
}

func commandName(operationID string) string {
	runes := []rune(operationID)
	var builder strings.Builder
	for index, current := range runes {
		if current == '_' || current == ' ' {
			if builder.Len() > 0 {
				builder.WriteByte('-')
			}
			continue
		}
		if index > 0 && unicode.IsUpper(current) {
			previous := runes[index-1]
			nextIsLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
			if previous != '_' && previous != ' ' &&
				(unicode.IsLower(previous) || unicode.IsDigit(previous) ||
					(unicode.IsUpper(previous) && nextIsLower)) {
				builder.WriteByte('-')
			}
		}
		builder.WriteRune(unicode.ToLower(current))
	}
	return builder.String()
}

func addOperationParameter(options *callOptions, name string, value any) error {
	parameter, ok := findOperationParameter(options.operation, name)
	if !ok {
		return fmt.Errorf("operation %s has no parameter %q", options.operation.ID, name)
	}
	addParameterValue(options, parameter, value)
	return nil
}

func addParameterValue(
	options *callOptions,
	parameter clicontract.Parameter,
	value any,
) {
	switch parameter.In {
	case "path":
		addValue(options.path, parameter.Name, value)
	case "query":
		addValue(options.query, parameter.Name, value)
	case "header":
		addValue(options.headers, parameter.Name, value)
	}
}

func findOperationParameter(
	operation clicontract.Descriptor,
	flag string,
) (clicontract.Parameter, bool) {
	want := normalizeFlagName(flag)
	for _, parameter := range operation.Parameters {
		if normalizeFlagName(parameter.Name) == want {
			return parameter, true
		}
	}
	return clicontract.Parameter{}, false
}

func consumeLeadingJSONFlag(arguments []string, output outputMode) ([]string, outputMode) {
	result := arguments
	for len(result) > 0 && result[0] == "--json" {
		output = outputJSON
		result = result[1:]
	}
	return result, output
}

func modeFromOptions(defaultMode outputMode, options humanOptions) outputMode {
	if options.jsonOutput {
		return outputJSON
	}
	return defaultMode
}

type parsedOptionValue struct {
	value string
	index int
}

func optionValue(
	arguments []string,
	index int,
	inlineValue string,
	hasInlineValue bool,
	name string,
) (parsedOptionValue, error) {
	if hasInlineValue {
		return parsedOptionValue{value: inlineValue, index: index + 1}, nil
	}
	if index+1 >= len(arguments) || strings.HasPrefix(arguments[index+1], "--") {
		return parsedOptionValue{}, fmt.Errorf("--%s requires a value", name)
	}
	return parsedOptionValue{value: arguments[index+1], index: index + 2}, nil
}

type parsedBoolOption struct {
	value bool
	index int
}

func boolOption(
	index int,
	inlineValue string,
	hasInlineValue bool,
	name string,
	defaultValue bool,
) (parsedBoolOption, error) {
	if !hasInlineValue {
		return parsedBoolOption{value: defaultValue, index: index + 1}, nil
	}
	parsed, err := strconv.ParseBool(inlineValue)
	if err != nil {
		return parsedBoolOption{}, fmt.Errorf("--%s must be true or false", name)
	}
	return parsedBoolOption{value: parsed, index: index + 1}, nil
}

func splitLongFlag(argument string) (string, string, bool, bool) {
	if !strings.HasPrefix(argument, "--") || argument == "--" {
		return "", "", false, false
	}
	raw := strings.TrimPrefix(argument, "--")
	name, value, found := strings.Cut(raw, "=")
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", false, false
	}
	return name, value, found, true
}

func parseKeyValue(raw string) (string, any, error) {
	key, value, err := parseStringKeyValue(raw)
	if err != nil {
		return "", nil, err
	}
	return key, parseCLIValue(value), nil
}

func parseStringKeyValue(raw string) (string, string, error) {
	key, value, found := strings.Cut(raw, "=")
	key = strings.TrimSpace(key)
	if !found || key == "" {
		return "", "", fmt.Errorf("expected key=value, got %q", raw)
	}
	return key, value, nil
}

func parseCLIValue(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var parsed any
	if err := decoder.Decode(&parsed); err == nil {
		var trailing any
		if errors.Is(decoder.Decode(&trailing), io.EOF) {
			return parsed
		}
	}
	return value
}

func readJSONArgument(value string) (json.RawMessage, error) {
	if path, found := strings.CutPrefix(value, "@"); found {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return raw, nil
	}
	return json.RawMessage(value), nil
}

func parseCollaborationMessage(raw string) (wikicli.CollaborationMessage, error) {
	content, err := readJSONArgument(raw)
	if err != nil {
		return wikicli.CollaborationMessage{}, fmt.Errorf("read message: %w", err)
	}
	var message wikicli.CollaborationMessage
	if err := json.Unmarshal(content, &message); err != nil {
		return wikicli.CollaborationMessage{}, fmt.Errorf("decode message: %w", err)
	}
	return message, nil
}

func parseCollaborationMessages(raw string) ([]wikicli.CollaborationMessage, error) {
	content, err := readJSONArgument(raw)
	if err != nil {
		return nil, fmt.Errorf("read messages: %w", err)
	}
	var messages []wikicli.CollaborationMessage
	if err := json.Unmarshal(content, &messages); err != nil {
		return nil, fmt.Errorf("decode messages: %w", err)
	}
	return messages, nil
}

func addValue(values map[string]any, key string, value any) {
	if existing, ok := values[key]; ok {
		if list, ok := existing.([]any); ok {
			values[key] = append(list, value)
		} else {
			values[key] = []any{existing, value}
		}
		return
	}
	values[key] = value
}

func nilIfEmpty(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	return values
}

func nilIfEmptyString(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return values
}

func normalizeFlagName(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, "-", "_"))
}

func isHelpToken(value string) bool {
	return value == "--help" || value == "-h"
}

func containsHelpToken(values []string) bool {
	for _, value := range values {
		if isHelpToken(value) {
			return true
		}
	}
	return false
}

func unexpectedArgument(value string) error {
	return fmt.Errorf("unexpected argument %q", value)
}

func unknownFlag(name string) error {
	return fmt.Errorf("unknown flag --%s", name)
}

func writeStartupError(mode outputMode, err error) {
	if mode == outputJSON {
		_ = wikicli.EncodeResult(os.Stdout, wikicli.Result{
			OK: false, Action: "startup",
			Error: &wikicli.Error{
				Code: "invalid_arguments", Message: err.Error(),
			},
		})
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "error: %s\n", err)
}

func writeHumanResult(
	stdout io.Writer,
	stderr io.Writer,
	result wikicli.Result,
) error {
	if !result.OK {
		if result.Error == nil {
			_, err := fmt.Fprintln(stderr, "error: command failed")
			return err
		}
		_, err := fmt.Fprintf(
			stderr, "error: %s: %s\n",
			result.Error.Code, result.Error.Message,
		)
		if err != nil {
			return err
		}
		if result.Error.Details != nil {
			return writePrettyJSON(stderr, result.Error.Details)
		}
		return nil
	}

	switch result.Action {
	case "version":
		return writeVersion(stdout, result.Data)
	case "operations.list":
		return writeOperationList(stdout, result.Data)
	case "operation.describe":
		return writeOperationDescription(stdout, result.Data)
	case "config.show":
		return writeConfig(stdout, result.Data)
	case "auth.exchange":
		return writeAuthExchange(stdout, result.Data)
	case "auth.logout":
		return writeAuthLogout(stdout, result.Data)
	default:
		if result.Data == nil {
			_, err := fmt.Fprintln(stdout, "ok")
			return err
		}
		return writePrettyJSON(stdout, result.Data)
	}
}

func writeVersion(stdout io.Writer, data any) error {
	object, ok := data.(map[string]string)
	if ok {
		_, err := fmt.Fprintln(stdout, object["version"])
		return err
	}
	return writePrettyJSON(stdout, data)
}

func writeOperationList(stdout io.Writer, data any) error {
	var payload struct {
		Items []struct {
			OperationID string   `json:"operation_id"`
			Method      string   `json:"method"`
			Path        string   `json:"path"`
			Summary     string   `json:"summary"`
			Tags        []string `json:"tags"`
		} `json:"items"`
		Count int `json:"count"`
	}
	if !decodeData(data, &payload) {
		return writePrettyJSON(stdout, data)
	}
	writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "COMMAND\tOPERATION\tMETHOD\tPATH\tSUMMARY")
	for _, item := range payload.Items {
		_, _ = fmt.Fprintf(
			writer, "%s\t%s\t%s\t%s\t%s\n",
			commandName(item.OperationID), item.OperationID,
			item.Method, item.Path, item.Summary,
		)
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "\n%d operation(s)\n", payload.Count)
	return err
}

func writeOperationDescription(stdout io.Writer, data any) error {
	var operation clicontract.Descriptor
	if !decodeData(data, &operation) {
		return writePrettyJSON(stdout, data)
	}
	writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(writer, "Command:\t%s\n", commandName(operation.ID))
	_, _ = fmt.Fprintf(writer, "Operation:\t%s\n", operation.ID)
	_, _ = fmt.Fprintf(writer, "Method:\t%s\n", operation.Method)
	_, _ = fmt.Fprintf(writer, "Path:\t%s\n", operation.Path)
	if operation.Summary != "" {
		_, _ = fmt.Fprintf(writer, "Summary:\t%s\n", operation.Summary)
	}
	if len(operation.Tags) > 0 {
		_, _ = fmt.Fprintf(writer, "Tags:\t%s\n", strings.Join(operation.Tags, ", "))
	}
	if len(operation.Parameters) > 0 {
		_, _ = fmt.Fprintln(writer, "\nParameters:")
		_, _ = fmt.Fprintln(writer, "NAME\tIN\tREQUIRED")
		for _, parameter := range operation.Parameters {
			_, _ = fmt.Fprintf(
				writer, "%s\t%s\t%t\n",
				parameter.Name, parameter.In, parameter.Required,
			)
		}
	}
	if operation.RequestBody != nil {
		_, _ = fmt.Fprintf(
			writer, "\nRequest body:\trequired=%t content=%s\n",
			operation.RequestBody.Required,
			strings.Join(operation.RequestBody.ContentTypes, ", "),
		)
	}
	if len(operation.Responses) > 0 {
		_, _ = fmt.Fprintln(writer, "\nResponses:")
		_, _ = fmt.Fprintln(writer, "STATUS\tCONTENT")
		for _, response := range operation.Responses {
			_, _ = fmt.Fprintf(
				writer, "%s\t%s\n",
				response.Status, strings.Join(response.ContentTypes, ", "),
			)
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if operation.RequestBody != nil {
		_, err := fmt.Fprintf(
			stdout,
			"\nUse direct flags, --field key=value, --body '{...}', or --body @file.json.\n",
		)
		return err
	}
	return nil
}

func writeConfig(stdout io.Writer, data any) error {
	object, ok := data.(map[string]any)
	if !ok {
		return writePrettyJSON(stdout, data)
	}
	writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for _, key := range []string{
		"config_path", "base_url", "authenticated", "token_prefix",
		"actor_id", "display_name", "expires_at",
	} {
		if value, exists := object[key]; exists {
			_, _ = fmt.Fprintf(writer, "%s:\t%v\n", key, value)
		}
	}
	return writer.Flush()
}

func writeAuthExchange(stdout io.Writer, data any) error {
	object, ok := data.(map[string]any)
	if !ok {
		return writePrettyJSON(stdout, data)
	}
	writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "Authenticated")
	for _, key := range []string{
		"display_name", "actor_id", "token_prefix", "expires_at",
		"persisted", "config_path", "token",
	} {
		if value, exists := object[key]; exists {
			_, _ = fmt.Fprintf(writer, "%s:\t%v\n", key, value)
		}
	}
	return writer.Flush()
}

func writeAuthLogout(stdout io.Writer, data any) error {
	object, ok := data.(map[string]any)
	if !ok {
		return writePrettyJSON(stdout, data)
	}
	writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "Logged out")
	for _, key := range []string{"revoked", "local_credentials_removed", "config_path"} {
		if value, exists := object[key]; exists {
			_, _ = fmt.Fprintf(writer, "%s:\t%v\n", key, value)
		}
	}
	return writer.Flush()
}

func writePrettyJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func decodeData(data any, target any) bool {
	raw, err := json.Marshal(data)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, target) == nil
}

func humanHelp() string {
	return `Anby Wiki CLI

JSON agent mode:
  anby-wiki < request.json
  anby-wiki --input request.json

Human mode:
  anby-wiki auth exchange CODE --base-url https://anbywiki.example.com
  anby-wiki auth status
  anby-wiki auth logout
  anby-wiki config show
  anby-wiki operations list [--tag pages] [--search title]
  anby-wiki describe create-page
  anby-wiki collaboration run --page-id PAGE_ID --client-id CLIENT_ID
  anby-wiki call create-page --namespace main --title 'CLI Page'
  anby-wiki get-page-by-id --id PAGE_ID

All OpenAPI operations are available as kebab-case commands, or through
"call OPERATION". Use --json with any human command to print the full JSON
envelope.

Global flags:
  --base-url URL       Override configured server URL.
  --config PATH        Override local config path.
  --timeout SECONDS    Override request timeout.
  --json               Print the JSON result envelope.

Call flags:
  --path key=value     Explicit path parameter.
  --query key=value    Query parameter; repeat for arrays.
  --header key=value   Extra request header.
  --param key=value    Route an OpenAPI parameter to path/query/header.
  --field key=value    Add a top-level JSON or multipart body field.
  --body JSON          Use a raw JSON body.
  --body @file.json    Read raw JSON body from file.
  --body-file PATH     Read raw JSON body from file.
  --file key=PATH      Attach a multipart file field.

Use "anby-wiki operations list" to discover commands and
"anby-wiki describe COMMAND" to inspect required parameters and body schemas.
`
}

func authHelp() string {
	return `Usage:
  anby-wiki auth exchange CODE --base-url URL
  anby-wiki auth status
  anby-wiki auth logout

Flags:
  --code CODE          One-time code from /settings/cli.
  --no-persist         Print token instead of writing local config.
  --base-url URL       Override configured server URL.
  --config PATH        Override local config path.
  --timeout SECONDS    Override request timeout.
  --json               Print the JSON result envelope.
`
}

func configHelp() string {
	return `Usage:
  anby-wiki config show [--config PATH] [--json]
`
}

func operationsHelp() string {
	return `Usage:
  anby-wiki operations list [--tag TAG] [--search TEXT] [--json]
  anby-wiki operations describe COMMAND [--json]
`
}

func describeHelp() string {
	return `Usage:
  anby-wiki describe COMMAND [--json]

COMMAND may be an OpenAPI operationId such as createPage, or its kebab-case
alias such as create-page.
`
}

func callHelp() string {
	return `Usage:
  anby-wiki call COMMAND [flags]
  anby-wiki COMMAND [flags]

Examples:
  anby-wiki get-page --id PAGE_ID
  anby-wiki create-page --namespace main --title 'CLI Page' --language zh-Hans --content-model block-v1
  anby-wiki create-import-upload-job --title 'Import source' --route-mode auto --file file=./source.pdf --timeout 600

Use direct flags or --field for simple top-level object fields, and --body
for complex payloads.
`
}

func collaborationHelp() string {
	return `Usage:
  anby-wiki collaboration run --page-id PAGE_ID --client-id CLIENT_ID [flags]

Flags:
  --page-id ID          Page UUID to recover.
  --client-id ID        Collaboration client UUID.
  --last-sequence N     Last durable sequence known by the client.
  --message JSON        One collaboration message object; repeatable.
  --messages JSON       Array of collaboration message objects.
  --message @file.json  Read one message from file.
  --messages @file.json Read message array from file.
  --timeout SECONDS     Override request timeout.
  --json                Print the JSON result envelope.
`
}
