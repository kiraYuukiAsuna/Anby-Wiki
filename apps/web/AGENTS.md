# apps/web 前端约定

> 根约定见仓库根 `AGENTS.md`，冲突时本文件优先。

- API 调用一律经 `lib/api.ts` 导出的生成客户端工厂函数；禁止手写 URL/DTO。重新生成：`make gen-client`（产物 `contracts/generated/typescript` 禁止手改）。
- 服务端数据缓存唯一入口是 SWR；Zustand 只保存编辑器会话、面板开关等未提交交互状态，禁止双份缓存同一服务端实体。
- 表单、编辑器输入与客户端边界用 Zod 校验；Toast 用 Sonner；图标用 lucide-react；命令面板/可搜索选择器用 cmdk；组件基线 shadcn/ui（交互优先复用 Radix 原语）。
- 块编辑器只经 `components/editor/block-editor.tsx` 使用（ADR-0005）；BlockNote ↔ AST v1 双向映射在 `lib/editor/`，业务代码禁止直接引用 BlockNote API；改动映射后必须跑 `lib/editor/` 往返测试套件。
- 字体固定 Geist（layout 已挂载）；不引入第二套图标/字体/状态库。
- 提交前本地必须通过：`npm run typecheck && npm run lint && npm run test && npm run build`。

<!-- BEGIN:nextjs-agent-rules -->
# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.
<!-- END:nextjs-agent-rules -->

