// 章节锚点 slug 生成（M3-T03，设计 §9）。
// slug 只是展示地址，不是权威身份（权威身份是 page_id + heading_block_id）。
package projection

import (
	"strconv"
	"unicode"
)

// emptySlugFallback 标题去噪后为空时的回落 slug。
const emptySlugFallback = "section"

// anchorSlug 从 heading 标题确定性生成锚点 slug（纯函数）：
//  1. Unicode 小写折叠（strings.ToLower 语义，逐 rune）；
//  2. 空白与连字符统一折叠为一个连字符（不产生前导/尾随连字符）；
//  3. 去除其余非「Unicode 字母/数字」字符（CJK 等 Unicode 字母按 unicode.IsLetter
//     判断保留，不转拼音；数字按 unicode.IsDigit 保留）；
//  4. 结果为空时回落 emptySlugFallback。
func anchorSlug(title string) string {
	var b []rune
	pendingHyphen := false
	for _, r := range title {
		switch {
		case unicode.IsSpace(r) || r == '-':
			pendingHyphen = true
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			r = unicode.ToLower(r)
			if pendingHyphen && len(b) > 0 {
				b = append(b, '-')
			}
			pendingHyphen = false
			b = append(b, r)
		default:
			// 去除标点、符号等其他字符
		}
	}
	if len(b) == 0 {
		return emptySlugFallback
	}
	return string(b)
}

// slugAssigner 同页 slug 去重：按文档顺序分配，重复标题依次加 -2/-3 后缀
// （含与「基础 slug 恰好带数字后缀」的标题碰撞的避让）。确定性：同一文档
// 同一遍历顺序产出同一组 slug。
type slugAssigner struct {
	used map[string]bool
}

func newSlugAssigner() *slugAssigner {
	return &slugAssigner{used: map[string]bool{}}
}

// assign 为标题分配页内唯一 slug。
func (a *slugAssigner) assign(title string) string {
	base := anchorSlug(title)
	slug := base
	for i := 2; a.used[slug]; i++ {
		slug = base + "-" + strconv.Itoa(i)
	}
	a.used[slug] = true
	return slug
}
