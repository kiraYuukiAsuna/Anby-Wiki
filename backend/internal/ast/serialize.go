// 稳定序列化与内容哈希。
//
// Canonical 规则（与 apps/web/lib/ast/serialize.ts 逐字节一致，供跨语言哈希）：
//   - 对象键按 UTF-8 字节序（即 Unicode 码点序）升序；v1 键全为 ASCII，
//     与 JS 按 UTF-16 码元序排序结果相同；
//   - 无任何空白；
//   - 字符串：仅转义 '"'、'\\' 与 < 0x20 的控制字符
//     （\b \f \n \r \t 用短转义，其余用 \u00xx 小写 hex），
//     其余字符（含非 ASCII 与 '<' '>' '&'）保留原始 UTF-8，不做 HTML 转义；
//   - 数字：v1 AST 只含整数（schema_version、level），按十进制整数输出；
//     非整数或非安全整数范围（|n| > 2^53-1）的数字直接报错，不参与 canonical 约定。
package ast

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// maxSafeInteger 是 IEEE 754 双精度可精确表示的最大整数（2^53 - 1）。
const maxSafeInteger = 9007199254740991

// CanonicalJSON 输出 doc 的确定性字节表示，供哈希与快照存储。
// 不依赖 Go map 迭代序；结构体经由 encoding/json 投影（omitempty 语义生效）
// 后按 canonical 规则重写。
func CanonicalJSON(doc *Document) ([]byte, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("ast: 序列化文档失败: %w", err)
	}
	return CanonicalizeJSON(raw)
}

// CanonicalizeJSON 将任意 JSON 字节重写为 canonical 形式（键排序、去空白、规范转义）。
// 输入必须是合法 JSON；语义相同的输入（键序、空白、转义方式不同）产出相同字节。
func CanonicalizeJSON(data []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("ast: 解析 JSON 失败: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("ast: JSON 含有多个顶层值")
	}
	var out []byte
	out, err := appendCanonical(out, v)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ContentHash 返回 SHA-256(CanonicalJSON(doc)) 的小写 hex 字符串。
func ContentHash(doc *Document) (string, error) {
	canon, err := CanonicalJSON(doc)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

func appendCanonical(dst []byte, v any) ([]byte, error) {
	switch t := v.(type) {
	case nil:
		return append(dst, "null"...), nil
	case bool:
		return strconv.AppendBool(dst, t), nil
	case string:
		return appendCanonicalString(dst, t), nil
	case json.Number:
		return appendCanonicalNumber(dst, t)
	case []any:
		dst = append(dst, '[')
		for i, e := range t {
			if i > 0 {
				dst = append(dst, ',')
			}
			var err error
			dst, err = appendCanonical(dst, e)
			if err != nil {
				return nil, err
			}
		}
		return append(dst, ']'), nil
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		// sort.Strings 按字节升序，等价于 Unicode 码点序。
		sort.Strings(keys)
		dst = append(dst, '{')
		for i, k := range keys {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendCanonicalString(dst, k)
			dst = append(dst, ':')
			var err error
			dst, err = appendCanonical(dst, t[k])
			if err != nil {
				return nil, err
			}
		}
		return append(dst, '}'), nil
	default:
		return nil, fmt.Errorf("ast: canonical 不支持 %T", v)
	}
}

func appendCanonicalNumber(dst []byte, n json.Number) ([]byte, error) {
	f, err := strconv.ParseFloat(n.String(), 64)
	if err != nil {
		return nil, fmt.Errorf("ast: 非法数字 %q: %w", n.String(), err)
	}
	if math.IsInf(f, 0) || math.IsNaN(f) || f != math.Trunc(f) || math.Abs(f) > maxSafeInteger {
		return nil, fmt.Errorf("ast: canonical 仅支持安全整数，得到 %q", n.String())
	}
	return strconv.AppendInt(dst, int64(f), 10), nil
}

func appendCanonicalString(dst []byte, s string) []byte {
	const hexDigits = "0123456789abcdef"
	dst = append(dst, '"')
	for _, r := range s {
		switch r {
		case '"':
			dst = append(dst, '\\', '"')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			if r < 0x20 {
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[r>>4], hexDigits[r&0xF])
			} else {
				// 含非 ASCII 与 '<' '>' '&'：保留原始 UTF-8，不转义。
				dst = append(dst, string(r)...)
			}
		}
	}
	return append(dst, '"')
}
