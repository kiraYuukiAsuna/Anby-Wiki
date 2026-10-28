# render — Typed Block AST v1 → 安全 HTML（M1-T06）

`RenderHTML(doc *ast.Document) (string, error)` 把 AST v1 文档渲染为 HTML 片段
（不含 `<html>/<head>`），供阅读 API（`GET /api/v1/pages/by-title`、`GET /api/v1/pages/{id}`）
随 `PageWithContent.content.html` 返回。渲染直连 ContentSnapshot；
M3 引入 RenderedPage 投影与缓存时复用本包，并按 `RendererVersion` 记录渲染版本。

## 渲染规则

### Block

| AST Block | HTML |
|---|---|
| `heading` (level 1–6) | `<hN id="{block_id}">…</hN>`；可折叠标题用 `<details>` |
| `paragraph` | `<p>…</p>` |
| `todo` / `toggle` | 禁用 checkbox + 正文 / `<details>` |
| `bullet_list` / `ordered_list` / `list_item` | `<ul>` / `<ol>` / `<li>`（容器，children 递归渲染） |
| `table` / `table_row` / `table_cell` | 带可选 caption、列头与行头语义的 `<table>` |
| `code` / `preformatted` / `equation` | 程序代码 / 固定宽度原文 / 块级公式，均支持可选 caption |
| `breadcrumb` | `<nav aria-label="Breadcrumb">`，稳定 Page ID 项输出站内链接 |
| `column_list` / `column` | 按 `width_ratio` 布局的多列容器；窄屏由 CSS 堆叠 |
| `button` | 带声明式动作数据的 `<button>`；渲染阶段不执行任何副作用 |
| `html` | 无脚本、无网络、无表单能力的 sandbox `srcdoc` iframe |
| `quote` | `<blockquote>…</blockquote>` |
| `callout` | `<div class="callout" data-callout="{kind}"><div class="callout-title">{kind}</div>…</div>` |
| `component` | 经冻结 ComponentVersion 与白名单 Registry 从 Entity/Claim 渲染；无 resolver 时输出无动态值的安全占位 |
| `image` / `video` / `audio` | 固定 AssetRevision 的图片、视频与音频播放器 |
| `file` / `pdf` | 不可变文件下载卡片 / 同源 PDF iframe 阅读器 |
| `bookmark` / `link_preview` | 冻结元数据的外部链接卡片；阅读时不回源 |
| `embed` | YouTube、Vimeo、Figma 白名单 sandbox iframe；其他地址退回安全链接 |
| `divider` | `<hr>` |

### Inline

| AST Inline | HTML |
|---|---|
| `text` | 转义文本；`marks` 按数组顺序由外向内叠加包裹：bold→`<strong>`、italic→`<em>`、underline→`<u>`、strikethrough→`<del>`、code→`<code>`；颜色输出白名单 `data-text-color` / `data-background-color` |
| `inline_code` | `<code>…</code>` |
| `page_reference`（已解析） | `<a href="/pages/{target_page_id}" data-page-ref>{display_text}</a>`；带 `target_heading_block_id` 时 href 追加 `#{block_id}` |
| `page_reference`（未解析） | `<a class="unresolved" data-unresolved-ref data-target-namespace data-target-title>{normalized_title}</a>`，**无 href，不可跳转** |
| `external_link` | `<a href="{url}" rel="noopener noreferrer nofollow" target="_blank">{display_text}</a>` |
| `entity_reference` | `<a href="/concepts/{entity_id}" data-entity-ref="{entity_id}">{display_text}</a>`；阅读时按主实体绑定解析到条目 |
| `claim_reference` | `<a href="/claims/{claim_id}" data-claim-ref="{claim_id}">{display_text}</a>` |
| `citation_reference` | `<sup id="cite-ref-{block_id}-{node_index}" data-citation-ref="{citation_id}" …><a href="#cite-note-{n}">[n]</a></sup>`；按全文 Citation 首次出现顺序编号，同一 ID 复用序号，并与文末 References 双向导航 |
| `mention` | `<span class="mention" data-actor-id="{actor_id}">@{display_text}</span>` |
| `date_mention` | `<time datetime="{date}" data-date-mention …>{display_text}</time>` |
| `database_mention` | `<a href="/datasets/{dataset_id}" data-database-mention>{display_text}</a>` |

未知 Block/Inline 类型、越界 heading level、无法解码的 content 返回 error，不做静默降级
（正常路径上文档入库前已经 `ast.ValidateJSON`，不会触发）。

## 安全规则

渲染输出不得包含任何脚本可执行面：

1. 所有文本节点与属性值一律经 `html.EscapeString` 转义（含 display_text、
   normalized_title、language、URL、Block ID）。
2. `external_link` 仅允许 `http`/`https` scheme（大小写不敏感，经 `net/url` 解析判定）；
   其余（`javascript:`、`data:`、`vbscript:` 等）**降级为纯文本渲染，不产生 `<a>`**。
3. 不输出任何事件属性（`on*`）或可执行 `<script>`；未解析引用不带 href。
4. `html` 是唯一 iframe 输出面：强制空 `sandbox`，内嵌 CSP 为
   `default-src 'none'`，仅允许内联样式及 `data:` 图片/字体，不授予脚本、网络、
   表单、同源或弹窗能力。
5. `button` 只输出经 base64url 编码的已校验动作。交互层二次确认后，插块走
   Proposal、记录更新走 Dataset 服务、Webhook 由公网 HTTPS 受控出口发送。
6. `bookmark` / `link_preview` 只渲染 Revision 中冻结的文本元数据；不注入远端
   HTML、脚本或图片。交互式 `embed` 必须同时通过 provider 与 URL 结构白名单，
   iframe 固定使用 cross-origin sandbox 与 CSP `frame-src` allowlist。

## 版本策略

`RendererVersion` 当前为 `"v13"`。旧投影会
因版本不匹配自动失效。规则：

- 任何影响输出字节流的变更（标签结构、属性、转义行为、URL 策略）都必须升版
  （`v2`、`v3`…），并在本节追加变更说明；
- 不影响输出的重构（性能、内部结构）不升版；
- M3 的 RenderedPage 投影按 `RendererVersion` 判断缓存是否需要重建，
  升版即触发全量重建，因此升版是显式决策，不随顺手修改发生；
- AST schema_version 升版（v2）时渲染器必须同步升版。

版本记录：

- `v13`：正文 EntityReference 保留实体 ID，链接进入条目解析路径；唯一条目跳转，多条目供读者选择。
- `v12`：增加 Underline、受限文本/背景颜色、DateMention 与 DatabaseMention。
- `v11`：增加 Audio、File、PDF、Bookmark、LinkPreview，并将 Embed 升级为
  YouTube/Vimeo/Figma 白名单交互式 iframe。
- `v10`：增加 Breadcrumb、多列、受治理页面按钮和 sandbox HTML Block。
- `v9`：增加可折叠标题、Todo、Toggle、Callout 自定义样式、语义表格、代码标题、
  固定宽度原文与块级公式。
- `v8`：Entity 标签按目标语言、`und`、其他主标签依次回退；多值 Claim 按 Property
  合并并链接目标 Entity，同时过滤历史自环 Claim、显示类型与待核验摘要。
- `v7`：Citation 使用全文稳定编号，正文 marker 链接文末 References，并为每次出现
  输出独立回链锚点；章节懒加载显式携带全文 Citation 顺序。
- `v6`：补齐媒体、数据视图、Embed、页内锚点、数学/mention，并由动态 resolver
  渲染 Claim 当前值。
- `v4`：ComponentBlock 通过受信任 Registry 渲染，缺少 resolver 时安全降级。
- `v3`（M4-T08）：ClaimReference、CitationReference 增加到只读详情页的站内链接。
- `v2`（M4-T06）：增加 EntityReference、ClaimReference、CitationReference HTML 与 Citation 文档内编号。
- `v1`：AST v1 基础 Block、页面引用与外链渲染。
