# ImportPlan v1

`plan.schema.json` 是 AI 来源导入的页面规划契约。它位于事实抽取之后、
ProposalOperation 合成之前，描述来源概况、一个或多个页面路由，以及带逐字证据的
Typed Block 草稿或补丁。

- `create` 可创建多个新页面；`update` 只能选择服务端提供的候选 Page ID；
- `append` 生成新 Block，`replace` 只能选择候选页面中提供的 Block ID；
- 模型不生成权威 Page/Block/Citation ID；服务端校验后统一预分配；
- 所有正文 Block 必须绑定 SourceChunk 内可逐字核验的 evidence；
- `link.related_to` 必须逐项指向同一计划内 create/update 路由的精确标题，并携带
  可核验 `evidence`；它会作为这些目标页的已解析 Typed AST 页面引用写入，反向关系由
  `page_link_projection` 重建。无有效来源路由的 link 会被丢弃，`ignore` 不产生写入。
