package importer

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/anby/wiki/backend/internal/evidence"
)

func parsePlainText(content []byte) ([]TextBlock, error) {
	content = []byte(strings.ReplaceAll(string(content), "\r\n", "\n"))
	if !utf8.Valid(content) {
		return nil, ErrParseFailed
	}
	lines := strings.Split(string(content), "\n")
	blocks := []TextBlock{}
	sectionPath := []string{}
	for index := 0; index < len(lines); {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			index++
			continue
		}
		if level, title, ok := plainHeading(line); ok {
			parentCount := min(level-1, len(sectionPath))
			sectionPath = append(
				append([]string(nil), sectionPath[:parentCount]...),
				title,
			)
			if level > 1 {
				appendPlainHint(&blocks, sectionPath, title, sourceBlockHint{
					BlockType: "heading", Text: title, Level: level,
				})
			}
			index++
			continue
		}
		if strings.HasPrefix(line, "```") {
			language := strings.TrimSpace(strings.TrimPrefix(line, "```"))
			start := index + 1
			index = start
			for index < len(lines) && strings.TrimSpace(lines[index]) != "```" {
				index++
			}
			text := strings.Join(lines[start:index], "\n")
			if strings.TrimSpace(text) != "" {
				if plainPreformattedLanguage(language) {
					appendPlainHint(&blocks, sectionPath, text, sourceBlockHint{
						BlockType: "preformatted", Text: text,
						Kind: sourcePreformattedKind(text),
					})
				} else {
					appendPlainHint(&blocks, sectionPath, text, sourceBlockHint{
						BlockType: "code", Text: text, Language: language,
					})
				}
			}
			if index < len(lines) {
				index++
			}
			continue
		}
		if checked, text, ok := plainTodo(line); ok {
			appendPlainHint(&blocks, sectionPath, text, sourceBlockHint{
				BlockType: "todo", Text: text, Checked: &checked,
			})
			index++
			continue
		}
		if item, ok := plainBulletItem(line); ok {
			items := []string{item}
			index++
			for index < len(lines) {
				item, ok = plainBulletItem(strings.TrimSpace(lines[index]))
				if !ok {
					break
				}
				items = append(items, item)
				index++
			}
			appendPlainHint(&blocks, sectionPath, strings.Join(items, "\n"), sourceBlockHint{
				BlockType: "bullet_list", Items: items,
			})
			continue
		}
		if item, ok := plainOrderedItem(line); ok {
			items := []string{item}
			index++
			for index < len(lines) {
				item, ok = plainOrderedItem(strings.TrimSpace(lines[index]))
				if !ok {
					break
				}
				items = append(items, item)
				index++
			}
			appendPlainHint(&blocks, sectionPath, strings.Join(items, "\n"), sourceBlockHint{
				BlockType: "ordered_list", Items: items,
			})
			continue
		}
		if strings.HasPrefix(line, ">") {
			items := []string{strings.TrimSpace(strings.TrimPrefix(line, ">"))}
			index++
			for index < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[index]), ">") {
				items = append(items, strings.TrimSpace(strings.TrimPrefix(
					strings.TrimSpace(lines[index]), ">",
				)))
				index++
			}
			if len(items) > 1 {
				marker := strings.ToUpper(strings.TrimSpace(items[0]))
				if marker == "[!NOTE]" || marker == "[!WARNING]" || marker == "[!DANGER]" {
					kind, color := "info", "blue"
					if marker == "[!WARNING]" {
						kind, color = "warning", "yellow"
					} else if marker == "[!DANGER]" {
						kind, color = "danger", "red"
					}
					text := strings.Join(items[1:], " ")
					appendPlainHint(&blocks, sectionPath, text, sourceBlockHint{
						BlockType: "callout", Text: text, Kind: kind, Color: color,
					})
					continue
				}
			}
			text := strings.Join(items, " ")
			appendPlainHint(&blocks, sectionPath, text, sourceBlockHint{
				BlockType: "quote", Text: text,
			})
			continue
		}
		if rows, consumed := plainTable(lines[index:]); consumed > 1 {
			textRows := make([]string, len(rows))
			for rowIndex := range rows {
				textRows[rowIndex] = strings.Join(rows[rowIndex], "\t")
			}
			appendPlainHint(&blocks, sectionPath, strings.Join(textRows, "\n"), sourceBlockHint{
				BlockType: "table", Rows: rows, HasColumnHeader: true,
			})
			index += consumed
			continue
		}
		if expression, ok := plainEquation(line); ok {
			appendPlainHint(&blocks, sectionPath, expression, sourceBlockHint{
				BlockType: "equation", Expression: expression,
			})
			index++
			continue
		}
		if title, body, consumed := plainToggle(lines[index:]); consumed > 0 {
			appendPlainHint(&blocks, sectionPath, title+"\n"+body, sourceBlockHint{
				BlockType: "toggle", Text: title,
				Children: []sourceBlockHint{{BlockType: "paragraph", Text: body}},
			})
			index += consumed
			continue
		}
		if kind, color, text, ok := plainCallout(line); ok {
			appendPlainHint(&blocks, sectionPath, text, sourceBlockHint{
				BlockType: "callout", Text: text, Kind: kind, Color: color,
			})
			index++
			continue
		}
		end := index
		for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
			end++
		}
		fixedWidth := strings.Join(lines[index:end], "\n")
		if len(detectPreformattedSpans([]rune(fixedWidth))) > 0 {
			blocks = append(blocks, TextBlock{
				Kind: evidence.SourceChunkTypeText, Text: fixedWidth,
				SectionPath: append([]string(nil), sectionPath...),
			})
			index = end
			continue
		}

		paragraph := []string{line}
		index++
		for index < len(lines) && strings.TrimSpace(lines[index]) != "" &&
			!plainSpecialLine(strings.TrimSpace(lines[index])) {
			paragraph = append(paragraph, strings.TrimSpace(lines[index]))
			index++
		}
		blocks = append(blocks, TextBlock{
			Kind:        evidence.SourceChunkTypeText,
			Text:        strings.Join(paragraph, " "),
			SectionPath: append([]string(nil), sectionPath...),
		})
	}
	if len(blocks) == 0 {
		return nil, ErrParseFailed
	}
	return blocks, nil
}

func appendPlainHint(
	blocks *[]TextBlock,
	sectionPath []string,
	text string,
	hint sourceBlockHint,
) {
	kind := evidence.SourceChunkTypeText
	if hint.BlockType == "table" {
		kind = evidence.SourceChunkTypeTable
	}
	*blocks = append(*blocks, TextBlock{
		Kind: kind, Text: text,
		SectionPath: append([]string(nil), sectionPath...),
		Content:     sourceBlockHintJSON(hint),
	})
}

func plainHeading(line string) (int, string, bool) {
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || !unicode.IsSpace(rune(line[level])) {
		return 0, "", false
	}
	title := strings.TrimSpace(line[level:])
	return level, title, title != ""
}

func plainTodo(line string) (bool, string, bool) {
	value := strings.TrimSpace(strings.TrimPrefix(line, "-"))
	if len(value) < 4 || value[0] != '[' || value[2] != ']' {
		return false, "", false
	}
	checked := value[1] == 'x' || value[1] == 'X'
	if value[1] != ' ' && !checked {
		return false, "", false
	}
	text := strings.TrimSpace(value[3:])
	return checked, text, text != ""
}

func plainBulletItem(line string) (string, bool) {
	if len(line) < 2 || (line[0] != '-' && line[0] != '*') ||
		!unicode.IsSpace(rune(line[1])) {
		return "", false
	}
	item := strings.TrimSpace(line[1:])
	return item, item != ""
}

func plainOrderedItem(line string) (string, bool) {
	index := 0
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	if index == 0 || index+1 >= len(line) || line[index] != '.' ||
		!unicode.IsSpace(rune(line[index+1])) {
		return "", false
	}
	item := strings.TrimSpace(line[index+1:])
	return item, item != ""
}

func plainTable(lines []string) ([][]string, int) {
	rows := [][]string{}
	consumed := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Count(line, "|") < 1 {
			break
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for cellIndex := range cells {
			cells[cellIndex] = strings.TrimSpace(cells[cellIndex])
		}
		if len(cells) < 2 || len(rows) > 0 && len(cells) != len(rows[0]) {
			break
		}
		consumed++
		if plainTableDivider(cells) {
			continue
		}
		rows = append(rows, cells)
	}
	return rows, consumed
}

func plainTableDivider(cells []string) bool {
	for _, cell := range cells {
		value := strings.Trim(cell, " :-")
		if value != "" {
			return false
		}
	}
	return true
}

func plainEquation(line string) (string, bool) {
	if strings.HasPrefix(line, "$$") && strings.HasSuffix(line, "$$") && len(line) > 4 {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "$$"), "$$")), true
	}
	lower := strings.ToLower(line)
	for _, prefix := range []string{"equation:", "equation："} {
		if strings.HasPrefix(lower, prefix) {
			expression := strings.TrimSpace(line[len(prefix):])
			return expression, expression != ""
		}
	}
	return "", false
}

func plainToggle(lines []string) (string, string, int) {
	if len(lines) < 2 {
		return "", "", 0
	}
	first := strings.TrimSpace(lines[0])
	second := strings.TrimSpace(lines[1])
	lowerFirst := strings.ToLower(first)
	lowerSecond := strings.ToLower(second)
	if !strings.HasPrefix(lowerFirst, "toggle title:") ||
		!strings.HasPrefix(lowerSecond, "toggle content:") {
		return "", "", 0
	}
	title := strings.TrimSpace(first[len("toggle title:"):])
	body := strings.TrimSpace(second[len("toggle content:"):])
	if title == "" || body == "" {
		return "", "", 0
	}
	return title, body, 2
}

func plainCallout(line string) (string, string, string, bool) {
	lower := strings.ToLower(line)
	cases := []struct {
		prefix, kind, color string
	}{
		{"warning:", "warning", "yellow"},
		{"warning：", "warning", "yellow"},
		{"danger:", "danger", "red"},
		{"danger：", "danger", "red"},
		{"note:", "info", "blue"},
		{"note：", "info", "blue"},
	}
	for _, item := range cases {
		if strings.HasPrefix(lower, item.prefix) {
			text := strings.TrimSpace(line[len(item.prefix):])
			return item.kind, item.color, text, text != ""
		}
	}
	return "", "", "", false
}

func plainSpecialLine(line string) bool {
	_, _, heading := plainHeading(line)
	_, _, todo := plainTodo(line)
	_, bullet := plainBulletItem(line)
	_, ordered := plainOrderedItem(line)
	_, equation := plainEquation(line)
	_, _, _, callout := plainCallout(line)
	return heading || todo || bullet || ordered || equation || callout ||
		strings.HasPrefix(line, "```") || strings.HasPrefix(line, ">") ||
		strings.Count(line, "|") >= 1 ||
		strings.HasPrefix(strings.ToLower(line), "toggle title:")
}

func plainPreformattedLanguage(language string) bool {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "ascii", "diagram", "preformatted", "fixed", "plain":
		return true
	default:
		return false
	}
}
