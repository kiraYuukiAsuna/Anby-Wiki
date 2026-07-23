# render — Typed Block AST v1 → 安全 HTML（M1-T06）

`RenderHTML(doc *ast.Document) (string, error)` 把 AST v1 文档渲染为 HTML 片段
（不含 `<html>/<head>`），供阅读 API（`GET /api/v1/pages/by-title`、`GET /api/v1/pages/{id}`）
随 `PageWithContent.content.html` 返回。渲染直连 ContentSnapshot；
M3 引入 RenderedPage 投影与缓存时复用本包，并按 `RendererVersion` 记录渲染版本。

## 渲染规则

### Block

| AST Block | HTML |
|---|---|
| `heading` (level 1–6) | `<hN id="{block_id}">…</hN>`（id 锚点 = Block ID，供章节深链） |
| `paragraph` | `<p>…</p>` |
| `bullet_list` / `ordered_list` / `list_item` | `<ul>` / `<ol>` / `<li>`（容器，children 递归渲染） |
| `table` / `table_row` / `table_cell` | `<table><tbody>…</tbody></table>` / `<tr>` / `<td>`（v1 不区分 thead，全部进 tbody） |
| `code` | `<pre><code class="language-{language}">…</code></pre>`（language 为空时省略 class） |
| `quote` | `<blockquote>…</blockquote>` |
| `callout` | `<div class="callout" data-callout="{kind}"><div class="callout-title">{kind}</div>…</div>` |
| `component` | 经冻结 ComponentVersion 与白名单 Registry 从 Entity/Claim 渲染；无 resolver 时输出无动态值的安全占位 |
| `divider` | `<hr>` |

### Inline

| AST Inline | HTML |
|---|---|
| `text` | 转义文本；`marks` 按数组顺序由外向内叠加包裹：bold→`<strong>`、italic→`<em>`、strikethrough→`<del>`、code→`<code>` |
| `inline_code` | `<code>…</code>` |
| `page_reference`（已解析） | `<a href="/pages/{target_page_id}" data-page-ref>{display_text}</a>`；带 `target_heading_block_id` 时 href 追加 `#{block_id}` |
| `page_reference`（未解析） | `<a class="unresolved" data-unresolved-ref data-target-namespace data-target-title>{normalized_title}</a>`，**无 href，不可跳转** |
| `external_link` | `<a href="{url}" rel="noopener noreferrer nofollow" target="_blank">{display_text}</a>` |
| `entity_reference` | `<a href="/entities/{entity_id}" data-entity-ref>{display_text}</a>` |
| `claim_reference` | `<a href="/claims/{claim_id}" data-claim-ref="{claim_id}">{display_text}</a>` |
| `citation_reference` | `<sup data-citation-ref="{citation_id}" title="{display_text 或 citation_id}"><a href="/citations/{citation_id}">[n]</a></sup>`；按 Citation 首次出现顺序编号，同一 ID 复用序号 |

未知 Block/Inline 类型、越界 heading level、无法解码的 content 返回 error，不做静默降级
（正常路径上文档入库前已经 `ast.ValidateJSON`，不会触发）。

## 安全规则

渲染输出不得包含任何脚本可执行面：

1. 所有文本节点与属性值一律经 `html.EscapeString` 转义（含 display_text、
   normalized_title、language、URL、Block ID）。
2. `external_link` 仅允许 `http`/`https` scheme（大小写不敏感，经 `net/url` 解析判定）；
   其余（`javascript:`、`data:`、`vbscript:` 等）**降级为纯文本渲染，不产生 `<a>`**。
3. 不输出任何事件属性（`on*`）、`<script>`、`<iframe>` 等元素；
   未解析引用不带 href，杜绝构造跳转。

XSS 回归用例见 `render_test.go`：`<script>` 文本、`onerror` 属性注入、
`javascript:` URL、`data:` URL、URL/属性中的引号逃逸，以及知识引用展示文本/标题转义。

## 版本策略

`RendererVersion` 当前为 `"v3"`。v3 让 Claim/Citation 引用可导航到只读详情页。规则：

- 任何影响输出字节流的变更（标签结构、属性、转义行为、URL 策略）都必须升版
  （`v2`、`v3`…），并在本节追加变更说明；
- 不影响输出的重构（性能、内部结构）不升版；
- M3 的 RenderedPage 投影按 `RendererVersion` 判断缓存是否需要重建，
  升版即触发全量重建，因此升版是显式决策，不随顺手修改发生；
- AST schema_version 升版（v2）时渲染器必须同步升版。

版本记录：

- `v3`（M4-T08）：ClaimReference、CitationReference 增加到只读详情页的站内链接。
- `v2`（M4-T06）：增加 EntityReference、ClaimReference、CitationReference HTML 与 Citation 文档内编号。
- `v1`：AST v1 基础 Block、页面引用与外链渲染。
