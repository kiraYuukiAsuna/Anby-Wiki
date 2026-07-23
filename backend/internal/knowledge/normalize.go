package knowledge

import (
	"errors"
	"fmt"

	"golang.org/x/text/unicode/norm"

	"github.com/anby/wiki/backend/internal/page"
)

// 实体名/别名/canonical_key 的规范化规则与 page 标题完全一致（设计 §6.2 同 §5.1），
// 因此直接复用 page 包的实现，不复制规则：
//   - 规范化键：Unicode NFC → trim → 内部连续空白折叠 → 小写（page.NormalizeTitle）；
//   - 展示形态：trim + 折叠空白（page.DisplayTitle），本模块再补 NFC，
//     使落库标签经 lower() 后与规范化键严格可比（搜索 exact 匹配的 SQL 等价性依赖这一点）。
// page.ErrInvalidTitle 统一映射为本领域错误 ErrInvalidLabel。

// normalizeKey 计算 canonical_key / normalized_alias 的规范化键。
func normalizeKey(raw string) (string, error) {
	key, err := page.NormalizeTitle(raw)
	if err != nil {
		return "", mapLabelErr(err)
	}
	return key, nil
}

// displayLabel 计算标签/别名的落库展示形态：NFC + trim + 折叠空白（保留大小写）。
func displayLabel(raw string) (string, error) {
	display, err := page.DisplayTitle(raw)
	if err != nil {
		return "", mapLabelErr(err)
	}
	return norm.NFC.String(display), nil
}

// mapLabelErr 把 page 的标题错误翻译为 knowledge 的领域错误（双 %w 保留完整错误链）。
func mapLabelErr(err error) error {
	if errors.Is(err, page.ErrInvalidTitle) {
		return fmt.Errorf("%w: %w", ErrInvalidLabel, err)
	}
	return err
}
