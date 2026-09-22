// Command anby-wiki is the CLI client for Anby Wiki.
package main

import (
	"context"
	"os"

	"github.com/anby/wiki/backend/internal/wikicli"
)

var version = "dev"

func main() {
	invocation, argumentError := buildInvocation(os.Args[1:], os.Stdin)
	if invocation.closeInput != nil {
		defer invocation.closeInput()
	}
	if argumentError != nil {
		writeStartupError(invocation.output, argumentError)
		os.Exit(2)
	}
	if invocation.helpText != "" {
		_, _ = os.Stdout.WriteString(invocation.helpText)
		os.Exit(0)
	}
	input := wikicli.Input{}
	if invocation.input != nil {
		input = *invocation.input
	} else {
		decoded, err := wikicli.DecodeInput(invocation.reader)
		if err != nil {
			_ = wikicli.EncodeResult(os.Stdout, wikicli.Result{
				OK: false, Action: "startup",
				Error: &wikicli.Error{
					Code: "invalid_json", Message: err.Error(),
				},
			})
			os.Exit(2)
		}
		input = decoded
	}
	app, err := wikicli.New(version)
	if err != nil {
		_ = wikicli.EncodeResult(os.Stdout, wikicli.Result{
			OK: false, Action: input.Action,
			Error: &wikicli.Error{
				Code: "contract_error", Message: err.Error(),
			},
		})
		os.Exit(1)
	}
	result, exitCode := app.Execute(context.Background(), input)
	if invocation.output == outputJSON {
		if err := wikicli.EncodeResult(os.Stdout, result); err != nil {
			os.Exit(1)
		}
	} else {
		if err := writeHumanResult(os.Stdout, os.Stderr, result); err != nil {
			os.Exit(1)
		}
	}
	os.Exit(exitCode)
}
