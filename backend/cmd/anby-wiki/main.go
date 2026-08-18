// Command wiki-cli is the JSON-only agent client for Anby Wiki.
package main

import (
	"context"
	"io"
	"os"

	"github.com/anby/wiki/backend/internal/wikicli"
)

var version = "dev"

func main() {
	reader, closeInput, argumentError := inputReader(os.Args[1:])
	if closeInput != nil {
		defer closeInput()
	}
	if argumentError != nil {
		_ = wikicli.EncodeResult(os.Stdout, wikicli.Result{
			OK: false, Action: "startup",
			Error: &wikicli.Error{
				Code: "invalid_arguments", Message: argumentError.Error(),
			},
		})
		os.Exit(2)
	}
	input, err := wikicli.DecodeInput(reader)
	if err != nil {
		_ = wikicli.EncodeResult(os.Stdout, wikicli.Result{
			OK: false, Action: "startup",
			Error: &wikicli.Error{
				Code: "invalid_json", Message: err.Error(),
			},
		})
		os.Exit(2)
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
	if err := wikicli.EncodeResult(os.Stdout, result); err != nil {
		os.Exit(1)
	}
	os.Exit(exitCode)
}

func inputReader(arguments []string) (io.Reader, func(), error) {
	if len(arguments) == 0 {
		return os.Stdin, nil, nil
	}
	if len(arguments) != 2 || arguments[0] != "--input" {
		return nil, nil, &argumentError{
			message: "usage: wiki-cli [--input request.json]",
		}
	}
	file, err := os.Open(arguments[1])
	if err != nil {
		return nil, nil, err
	}
	return file, func() { _ = file.Close() }, nil
}

type argumentError struct {
	message string
}

func (e *argumentError) Error() string {
	return e.message
}
