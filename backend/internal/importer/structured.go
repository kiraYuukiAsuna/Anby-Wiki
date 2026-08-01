package importer

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const maxStructuredTextBytes = 24 << 20

func parseJSON(content []byte) ([]TextBlock, error) {
	content = bytes.TrimSpace(content)
	if !json.Valid(content) {
		return nil, ErrParseFailed
	}
	var normalized bytes.Buffer
	if err := json.Indent(&normalized, content, "", "  "); err != nil {
		return nil, ErrParseFailed
	}
	if normalized.Len() == 0 || normalized.Len() > maxStructuredTextBytes {
		return nil, ErrParseFailed
	}
	section := "JSON"
	return []TextBlock{{Text: normalized.String(), Section: &section}}, nil
}

func parseCSV(content []byte) ([]TextBlock, error) {
	if !utf8.Valid(content) {
		return nil, ErrParseFailed
	}
	reader := csv.NewReader(bytes.NewReader(content))
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false

	header, err := reader.Read()
	if err != nil {
		return nil, ErrParseFailed
	}
	if len(header) == 0 {
		return nil, ErrParseFailed
	}
	header[0] = strings.TrimPrefix(header[0], "\uFEFF")
	for index := range header {
		header[index] = strings.TrimSpace(header[index])
		if header[index] == "" {
			header[index] = fmt.Sprintf("column_%d", index+1)
		}
	}

	blocks := make([]TextBlock, 0)
	for rowNumber := 2; ; rowNumber++ {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, ErrParseFailed
		}
		values := make([]string, 0, max(len(header), len(record)))
		for index, raw := range record {
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			key := fmt.Sprintf("column_%d", index+1)
			if index < len(header) {
				key = header[index]
			}
			values = append(values, key+"="+value)
		}
		if len(values) == 0 {
			continue
		}
		section := fmt.Sprintf("CSV row %d", rowNumber)
		blocks = append(blocks, TextBlock{
			Text: strings.Join(values, "; "), Section: &section,
		})
	}
	if len(blocks) == 0 {
		section := "CSV header"
		blocks = append(blocks, TextBlock{
			Text: strings.Join(header, ", "), Section: &section,
		})
	}
	return blocks, nil
}
