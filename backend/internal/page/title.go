package page

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// MaxTitleLength 规范化标题的最大字符数（按 Unicode 码点计）。
const MaxTitleLength = 255

// NormalizeTitle 计算页面身份的规范化标题：
// Unicode NFC → 去首尾空白 → 内部连续空白折叠为单个空格 → 小写化。
// 空结果、超过 255 字符、含控制字符返回 ErrInvalidTitle。
// 纯函数，不依赖 DB。
func NormalizeTitle(raw string) (string, error) {
	folded, err := foldWhitespace(norm.NFC.String(raw))
	if err != nil {
		return "", err
	}
	return strings.ToLower(folded), nil
}

// DisplayTitle 计算显示标题：保留用户原始书写，只 trim + 折叠空白。
// 校验规则与 NormalizeTitle 一致（空、超长、控制字符同样拒绝）。
func DisplayTitle(raw string) (string, error) {
	return foldWhitespace(raw)
}

// foldWhitespace 去首尾空白并把内部连续空白折叠为单个空格，同时做合法性校验。
func foldWhitespace(s string) (string, error) {
	for _, r := range s {
		// 属于空白的控制字符（\t \n 等）交给下方折叠处理；
		// 其余控制字符（NUL、BEL 等）直接拒绝。
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return "", fmt.Errorf("%w: 含控制字符 U+%04X", ErrInvalidTitle, r)
		}
	}
	// strings.Fields 按 Unicode 空白切分（含全角空格 U+3000、NBSP 等），
	// Join 天然完成 trim + 折叠。
	folded := strings.Join(strings.Fields(s), " ")
	if folded == "" {
		return "", fmt.Errorf("%w: 空白标题", ErrInvalidTitle)
	}
	if utf8.RuneCountInString(folded) > MaxTitleLength {
		return "", fmt.Errorf("%w: 超过 %d 字符", ErrInvalidTitle, MaxTitleLength)
	}
	return folded, nil
}
