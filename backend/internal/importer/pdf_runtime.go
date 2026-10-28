package importer

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"regexp"
	"strconv"
)

const maxPDFInfoBytes = int64(1 << 20)

var pdfPagesPattern = regexp.MustCompile(`(?m)^Pages:\s+([0-9]+)\s*$`)

func pdfPageCount(
	ctx context.Context,
	pdfInfoPath string,
	content []byte,
) (int, error) {
	command := exec.CommandContext(ctx, pdfInfoPath, "-")
	command.Env = []string{}
	command.Stdin = bytes.NewReader(content)
	command.Stderr = io.Discard
	output, err := readBoundedCommandOutput(
		ctx, command, maxPDFInfoBytes, ErrParseFailed,
	)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, ErrParseFailed
	}
	match := pdfPagesPattern.FindSubmatch(output)
	if len(match) != 2 {
		return 0, ErrParseFailed
	}
	count, err := strconv.Atoi(string(match[1]))
	if err != nil || count <= 0 {
		return 0, ErrParseFailed
	}
	return count, nil
}

func readBoundedCommandOutput(
	ctx context.Context,
	command *exec.Cmd,
	maxBytes int64,
	tooLarge error,
) ([]byte, error) {
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxBytes+1))
	if int64(len(output)) > maxBytes {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, tooLarge
	}
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, readErr
	}
	if err := command.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return output, nil
}
