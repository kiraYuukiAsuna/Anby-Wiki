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
var pdfRotationPattern = regexp.MustCompile(`(?m)^Page\s+([0-9]+) rot:\s+([0-9]+)\s*$`)

func pdfPageRotations(ctx context.Context, pdfInfoPath string, content []byte, count int) ([]int, error) {
	command := exec.CommandContext(ctx, pdfInfoPath, "-f", "1", "-l", strconv.Itoa(count), "-")
	command.Env = []string{}
	command.Stdin = bytes.NewReader(content)
	command.Stderr = io.Discard
	output, err := readBoundedCommandOutput(ctx, command, maxPDFInfoBytes, ErrParseFailed)
	if err != nil {
		return nil, err
	}
	matches := pdfRotationPattern.FindAllSubmatch(output, -1)
	if len(matches) != count {
		return nil, ErrParseFailed
	}
	rotations := make([]int, count)
	for index, match := range matches {
		page, _ := strconv.Atoi(string(match[1]))
		rotation, _ := strconv.Atoi(string(match[2]))
		if page != index+1 || rotation > 270 || rotation%90 != 0 {
			return nil, ErrParseFailed
		}
		rotations[index] = rotation
	}
	return rotations, nil
}

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
