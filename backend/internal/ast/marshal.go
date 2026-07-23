// Block 的 JSON 序列化补丁：保留非 nil 空 children。
//
// ast.schema.json 要求容器 Block（list_item、quote、callout、table_cell、
// bullet_list、ordered_list、table、table_row）的 children 字段必须存在，
// 空数组合法；而 encoding/json 的 omitempty 会把空切片一并省略，
// 导致「删除容器最后一个子块」这类合法状态无法序列化为合法文档。
// 这里仅在 Children 非 nil 且为空时显式补回 "children":[]；
// Children 为 nil（divider 等无 children 类型）时行为与 omitempty 一致。
package ast

import "encoding/json"

// MarshalJSON 序列化 Block；非 nil 的空 Children 保留为 "children":[]。
func (b *Block) MarshalJSON() ([]byte, error) {
	type plain Block // 脱离方法集，避免递归
	raw, err := json.Marshal(plain(*b))
	if err != nil {
		return nil, err
	}
	if b.Children == nil || len(b.Children) > 0 {
		return raw, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	m["children"] = json.RawMessage("[]")
	return json.Marshal(m)
}
