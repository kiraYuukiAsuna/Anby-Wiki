# ProjectionApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getPageOutline**](ProjectionApi.md#getpageoutline) | **GET** /api/v1/pages/{id}/outline | 页面文档目录 |
| [**getPageReferences**](ProjectionApi.md#getpagereferences) | **GET** /api/v1/pages/{id}/references | 当前 Revision 的参考文献 |
| [**getPageSection**](ProjectionApi.md#getpagesection) | **GET** /api/v1/pages/{id}/sections/{section_key} | 按需读取一个当前章节分片 |
| [**getPageSections**](ProjectionApi.md#getpagesections) | **GET** /api/v1/pages/{id}/sections | 读取当前 Revision 的章节分片清单 |
| [**getRelatedPages**](ProjectionApi.md#getrelatedpages) | **GET** /api/v1/pages/{id}/related | 当前页面的相关文章 |
| [**listBacklinks**](ProjectionApi.md#listbacklinks) | **GET** /api/v1/pages/{id}/backlinks | 页面反向链接 |
| [**listCitationUsages**](ProjectionApi.md#listcitationusages) | **GET** /api/v1/citations/{id}/usages | Citation 页面使用位置 |
| [**listClaimUsages**](ProjectionApi.md#listclaimusages) | **GET** /api/v1/claims/{id}/usages | Claim 页面使用位置 |
| [**listComponentUsages**](ProjectionApi.md#listcomponentusages) | **GET** /api/v1/components/{id}/usages | Component 页面依赖位置 |
| [**listEntityMentions**](ProjectionApi.md#listentitymentions) | **GET** /api/v1/entities/{id}/mentions | Entity 页面提及位置 |
| [**listSourceUsages**](ProjectionApi.md#listsourceusages) | **GET** /api/v1/sources/{id}/usages | Source 页面使用位置 |
| [**locatePageSection**](ProjectionApi.md#locatepagesection) | **GET** /api/v1/pages/{id}/section-locator/{block_id} | 将稳定 Block ID 定位到其章节分片 |
| [**resolvePageAnchor**](ProjectionApi.md#resolvepageanchor) | **GET** /api/v1/pages/{id}/anchors/{slug} | 解析当前或历史章节锚点 |



## getPageOutline

> DocumentOutline getPageOutline(id)

页面文档目录

投影查询端点（匿名可读，无需登录）。返回文档大纲（heading 层级树）， 含层级、纯文本标题、锚点 slug 与序号路径 position_key，供阅读页 TOC 与锚点跳转。 数据来自 document_outline_projection / page_anchor 投影表（Worker 异步构建，最终一致）。 页面不存在返回 404；软删除页返回 410 gone；页面未发布过返回 200 且 items 为空数组。

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { GetPageOutlineRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetPageOutlineRequest;

  try {
    const data = await api.getPageOutline(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | 页面 ID（UUIDv7） | [Defaults to `undefined`] |

### Return type

[**DocumentOutline**](DocumentOutline.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 文档目录（items 按文档顺序） |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **410** | 资源曾存在但已删除（如软删除页面、重定向目标已删除） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getPageReferences

> PageReferenceList getPageReferences(id)

当前 Revision 的参考文献

匿名读取由 CitationUsage 投影生成的参考文献列表。编号按 Citation 在当前 Revision 中首次出现的文档顺序生成，相同 Citation 的多次引用共享编号。 ready&#x3D;false 表示 Worker 尚未完成当前 Revision 的投影，客户端不得将其解释为空引用。

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { GetPageReferencesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetPageReferencesRequest;

  try {
    const data = await api.getPageReferences(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | 页面 ID（UUIDv7） | [Defaults to `undefined`] |

### Return type

[**PageReferenceList**](PageReferenceList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前 Revision 的参考文献列表 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **410** | 资源曾存在但已删除（如软删除页面、重定向目标已删除） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getPageSection

> RenderedPageSection getPageSection(id, sectionKey)

按需读取一个当前章节分片

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { GetPageSectionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string
    sectionKey: sectionKey_example,
  } satisfies GetPageSectionRequest;

  try {
    const data = await api.getPageSection(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | 页面 ID（UUIDv7） | [Defaults to `undefined`] |
| **sectionKey** | `string` |  | [Defaults to `undefined`] |

### Return type

[**RenderedPageSection**](RenderedPageSection.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | AST 分片与安全 HTML 缓存 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **410** | 资源曾存在但已删除（如软删除页面、重定向目标已删除） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getPageSections

> PageSectionManifest getPageSections(id)

读取当前 Revision 的章节分片清单

匿名读取可重建的章节投影。仅当所有行匹配 Page.current_revision_id 与当前 renderer_version 时 ready&#x3D;true；否则返回 ready&#x3D;false，客户端应使用全文阅读。

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { GetPageSectionsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetPageSectionsRequest;

  try {
    const data = await api.getPageSections(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | 页面 ID（UUIDv7） | [Defaults to `undefined`] |

### Return type

[**PageSectionManifest**](PageSectionManifest.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前章节分片清单 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **410** | 资源曾存在但已删除（如软删除页面、重定向目标已删除） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getRelatedPages

> RelatedPageList getRelatedPages(id)

当前页面的相关文章

匿名读取可重建的相关文章投影。排序综合正文链接、反向链接、双向链接、 Collection、主 Entity 与 Entity 图谱关系，并返回可解释的匹配原因。

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { GetRelatedPagesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetRelatedPagesRequest;

  try {
    const data = await api.getRelatedPages(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | 页面 ID（UUIDv7） | [Defaults to `undefined`] |

### Return type

[**RelatedPageList**](RelatedPageList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 相关文章（按相关度降序，最多 12 条） |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **410** | 资源曾存在但已删除（如软删除页面、重定向目标已删除） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listBacklinks

> BacklinkListPage listBacklinks(id, cursor, pageSize)

页面反向链接

投影查询端点（匿名可读，无需登录）。列出指向该页面的已解析页面引用来源： 来源页（id/标题）+ 所在 Block + 展示文本。数据来自 page_link_projection 投影表 （设计 §17.3：关系查询走投影表，不扫 AST），由 Worker 异步构建， 新发布内容与投影之间存在最终一致窗口。 按 (source_page_id, source_block_id, source_node_id) 升序游标分页： 首页不传 cursor；后续页传上一页响应的 next_cursor；next_cursor 为 null 表示没有更多。 游标无法解析返回 400 validation_failed；页面不存在返回 404；软删除页返回 410 gone。

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { ListBacklinksRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListBacklinksRequest;

  try {
    const data = await api.listBacklinks(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | 页面 ID（UUIDv7） | [Defaults to `undefined`] |
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**BacklinkListPage**](BacklinkListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 一页反向链接 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **410** | 资源曾存在但已删除（如软删除页面、重定向目标已删除） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listCitationUsages

> ReferenceUsageListPage listCitationUsages(id, cursor, pageSize)

Citation 页面使用位置

匿名反向查询 CitationReference 的页面使用位置。只读 citation_usage，不扫描 AST JSON； 每条结果携带来源 Revision、Block、Node 与可选 Claim 上下文。

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { ListCitationUsagesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListCitationUsagesRequest;

  try {
    const data = await api.listCitationUsages(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**ReferenceUsageListPage**](ReferenceUsageListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 一页 Citation 使用位置 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listClaimUsages

> ReferenceUsageListPage listClaimUsages(id, cursor, pageSize)

Claim 页面使用位置

匿名反向查询 ClaimReference 的页面使用位置。只读 claim_usage，不扫描 AST JSON； 每条结果携带来源 Revision、Block 与 Node。

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { ListClaimUsagesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListClaimUsagesRequest;

  try {
    const data = await api.listClaimUsages(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**ReferenceUsageListPage**](ReferenceUsageListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 一页 Claim 使用位置 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listComponentUsages

> ComponentUsageListPage listComponentUsages(id, version, cursor, pageSize)

Component 页面依赖位置

匿名反向查询当前页面引用一个 Component 的位置。只读 component_dependency，不扫描 AST JSON；可按不可变版本过滤。

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { ListComponentUsagesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // number (optional)
    version: 56,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListComponentUsagesRequest;

  try {
    const data = await api.listComponentUsages(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **version** | `number` |  | [Optional] [Defaults to `undefined`] |
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**ComponentUsageListPage**](ComponentUsageListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 一页 Component 使用位置 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listEntityMentions

> ReferenceUsageListPage listEntityMentions(id, cursor, pageSize)

Entity 页面提及位置

匿名反向查询 EntityReference 的页面使用位置。只读 entity_mention_projection， 不扫描 AST JSON；每条结果携带来源 Revision、Block、Node 与 mention_text。

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { ListEntityMentionsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListEntityMentionsRequest;

  try {
    const data = await api.listEntityMentions(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**ReferenceUsageListPage**](ReferenceUsageListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 一页 Entity 提及位置 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listSourceUsages

> SourceUsageListPage listSourceUsages(id, cursor, pageSize)

Source 页面使用位置

匿名反向查询一个 Source 经 SourceVersion、Citation 被当前页面使用的位置。 只读 citation_usage 及证据身份链，不扫描 AST JSON。

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { ListSourceUsagesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListSourceUsagesRequest;

  try {
    const data = await api.listSourceUsages(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**SourceUsageListPage**](SourceUsageListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 一页 Source 使用位置 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## locatePageSection

> PageSectionLocator locatePageSection(id, blockId)

将稳定 Block ID 定位到其章节分片

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { LocatePageSectionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string
    blockId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies LocatePageSectionRequest;

  try {
    const data = await api.locatePageSection(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | 页面 ID（UUIDv7） | [Defaults to `undefined`] |
| **blockId** | `string` |  | [Defaults to `undefined`] |

### Return type

[**PageSectionLocator**](PageSectionLocator.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Block 所在的当前章节 key |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **410** | 资源曾存在但已删除（如软删除页面、重定向目标已删除） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## resolvePageAnchor

> AnchorTarget resolvePageAnchor(id, slug)

解析当前或历史章节锚点

匿名读取端点。先匹配当前 page_anchor slug，再匹配持久 page_anchor_alias， 最后跟随显式 BlockRedirect，返回目标页面的当前 slug 与稳定 Heading Block ID。 该端点用于章节改名或跨页面移动后的旧链接兼容，不扫描历史 AST。

### Example

```ts
import {
  Configuration,
  ProjectionApi,
} from '';
import type { ResolvePageAnchorRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ProjectionApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 当前或历史章节 slug
    slug: slug_example,
  } satisfies ResolvePageAnchorRequest;

  try {
    const data = await api.resolvePageAnchor(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **id** | `string` | 页面 ID（UUIDv7） | [Defaults to `undefined`] |
| **slug** | `string` | 当前或历史章节 slug | [Defaults to `undefined`] |

### Return type

[**AnchorTarget**](AnchorTarget.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 解析后的当前章节目标 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **410** | 资源曾存在但已删除（如软删除页面、重定向目标已删除） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

