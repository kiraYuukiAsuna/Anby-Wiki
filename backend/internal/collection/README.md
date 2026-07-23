# collection - Manual/Rule Collection 领域服务

`collection` 是 Collection 定义与物化 Membership 的唯一权威写入入口。
HTTP 层只暴露匿名列表、详情和成员查询；写入由领域服务校验 Actor、Wiki 边界、
规则引用与来源 Revision 后在单事务内完成。

## 模型与不变量

- Collection 仅支持 `manual` 与 `rule`；Manual 的 `query_json` 必须为空。
- Rule v1 仅支持 `entity_type` 和 `claim_exists`，未知字段、版本、类型或 Property 均拒绝。
- Manual 成员可指向同 Wiki 的 active Page 或 Entity；Rule 成员只指向 Entity。
- 每条 Membership 保存稳定 `sort_key`、`source_type` 和同 Wiki 的已发布 `source_revision_id`。
- Rule 重建在锁定 Collection 后全量替换成员，重复执行幂等，并删除不再匹配的成员。
- 查询只读 `collection_membership`，不得请求时扫描 AST 或执行任意查询。

## 分页

Collection 按 `(title, id)` 排序，Membership 按
`(sort_key, member_type, target_id)` 排序。游标是 URL-safe Base64 编码的内部键，
缺字段、空 UUID、未知成员类型或非法编码均返回 `ErrInvalidCursor`。

## 验证

```bash
TEST_DATABASE_URL=postgres://wiki@127.0.0.1:55432/wiki?sslmode=disable \
  go test ./internal/collection ./cmd/api ./migrations -count=1 -p 1
```
