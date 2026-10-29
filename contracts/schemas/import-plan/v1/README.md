# ImportPlan v1

`plan.schema.json` 是 AI 来源导入的页面规划契约。它位于事实抽取之后、
ProposalOperation 合成之前，描述来源概况、一个或多个页面路由，以及带逐字证据的
Typed Block 草稿或补丁。

- `create` 可创建多个新页面；`update` 只能选择服务端提供的候选 Page ID；
- `append` 生成新 Block，`replace` 只能选择候选页面中提供的 Block ID；
- update 路由固化规划时的 `planned_revision_id`，replace Block 固化
  `target_block_hash`；Composer 必须在生成 Proposal 前重新核验两者，拒绝覆盖规划后
  的人工修改；
- 模型不生成权威 Page/Block/Citation ID；服务端校验后统一预分配；
- 同批 create 路由由服务端预分配 Page ID，因此计划内页面关联可编译为稳定的已解析
  `page_reference`，不需要等待页面先落库；
- 所有正文 Block 必须绑定 SourceChunk 内可逐字核验的 evidence；
- create/update 路由的可选 `collections` 保存建议加入的现有手动专题、服务端固化
  的名称、理由、置信度与逐字证据。候选只来自当前 Wiki（最多 100 个），模型不得
  新建专题或推荐规则/动态集合；无证据、未知 ID 与重复推荐由服务端过滤/合并。
  旧计划缺少此字段时按空集合读取，不修改不可变计划记录。
- 专题归属随计划展示并确认，随后编译为最终人工审核 Proposal 中的
  `add_collection_membership`；与页面发布共享 ChangeBatch，可整批补偿回滚。
- `link.related_to` 必须逐项指向同一计划内 create/update 路由的精确标题，并携带
  可核验 `evidence`；它会作为这些目标页的已解析 Typed AST 页面引用写入，反向关系由
  `page_link_projection` 重建。无有效来源路由的 link 会被丢弃，`ignore` 不产生写入。
