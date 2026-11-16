# 页面翻译事件 v1

`page.translation_requested` 使用通用 Outbox envelope，aggregate_type 为 `page`，aggregate_id 为源页面 ID，payload 遵循 `requested.schema.json`。

事件固定源 Revision 和目标语言；`page_translation.event_id` 指向当前有效请求。Worker 重试沿用同一事件，人工重试替换事件 ID，旧任务不能发布。创建、发布译文与写入关联在一个领域事务内完成。已有译文不自动更新。

接口契约见 `contracts/openapi/openapi.yaml` 的 `PageTranslations` 和 `/api/v1/pages/{id}/translations`。默认阅读语言属于账号配置，不改变源页面的语言字段。

导入回滚在恢复页面的同一事务内删除未完成的活动翻译请求。Worker 提交时在源页面锁内重新读取活动请求，缺失或已被替换时丢弃结果。删除任一语言页面后，存活页面可建立新的翻译关联，旧页面的历史 Revision 保留。
