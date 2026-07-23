# HistoryApi

All URIs are relative to *http://localhost:8000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**diffRevisions**](HistoryApi.md#diffrevisions) | **GET** /api/v1/pages/{id}/diff | 两版结构 Diff |
| [**getRevision**](HistoryApi.md#getrevision) | **GET** /api/v1/pages/{id}/revisions/{rid} | 单版详情 |
| [**listRevisions**](HistoryApi.md#listrevisions) | **GET** /api/v1/pages/{id}/revisions | Revision 历史列表 |
| [**rollbackPage**](HistoryApi.md#rollbackpage) | **POST** /api/v1/pages/{id}/rollback | 回滚到历史版本 |



## diffRevisions

> DocumentDiff diffRevisions(id, from, to)

两版结构 Diff

阅读端点（匿名可读，无需登录）。以 from 为 base、to 为 current， 用后端 AST Diff（按 Block ID 对齐）计算结构差异：added/removed/changed/moved。 from &#x3D;&#x3D; to 返回空 changes。任一 Revision 不存在或不属于该页面返回 404。

### Example

```ts
import {
  Configuration,
  HistoryApi,
} from '';
import type { DiffRevisionsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new HistoryApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 基线 Revision ID（Diff 的 base 侧）
    from: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 目标 Revision ID（Diff 的 current 侧）
    to: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies DiffRevisionsRequest;

  try {
    const data = await api.diffRevisions(body);
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
| **from** | `string` | 基线 Revision ID（Diff 的 base 侧） | [Defaults to `undefined`] |
| **to** | `string` | 目标 Revision ID（Diff 的 current 侧） | [Defaults to `undefined`] |

### Return type

[**DocumentDiff**](DocumentDiff.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 结构 Diff（changes 确定性排序） |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getRevision

> RevisionDetail getRevision(id, rid)

单版详情

阅读端点（匿名可读，无需登录）。读取页面指定 Revision 的元信息与 canonical AST（不含 html）。Revision 不存在或不属于该页面返回 404 （跨页访问不泄露存在性）。

### Example

```ts
import {
  Configuration,
  HistoryApi,
} from '';
import type { GetRevisionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new HistoryApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | Revision ID（UUIDv7）
    rid: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetRevisionRequest;

  try {
    const data = await api.getRevision(body);
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
| **rid** | `string` | Revision ID（UUIDv7） | [Defaults to `undefined`] |

### Return type

[**RevisionDetail**](RevisionDetail.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Revision 元信息 + canonical AST |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listRevisions

> RevisionListPage listRevisions(id, cursor, pageSize)

Revision 历史列表

阅读端点（匿名可读，无需登录）。按 (created_at DESC, id DESC) 游标分页 列出页面 Revision 元信息（含冗余自快照的 content_hash/schema_version，不含 AST）。 首页不传 cursor；后续页传上一页响应的 next_cursor；next_cursor 为 null 表示没有更多。 游标无法解析返回 400 validation_failed；页面不存在返回 404。

### Example

```ts
import {
  Configuration,
  HistoryApi,
} from '';
import type { ListRevisionsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new HistoryApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListRevisionsRequest;

  try {
    const data = await api.listRevisions(body);
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

[**RevisionListPage**](RevisionListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 一页 Revision 历史 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## rollbackPage

> RollbackResponse rollbackPage(id, rollbackRequest)

回滚到历史版本

回滚不是修改旧 Revision（设计 §3.3）：以目标旧版快照内容复用发布事务 追加一个新 Revision（parent &#x3D; 当前 current），旧 Revision 与旧快照不动。 内容 hash 与历史版本相同的快照按 (content_hash, schema_version) 复用，不重复存储。 summary 缺省记录「回滚到 {target_revision_id}」，可由调用方覆盖。 审计事件为 revision.rolled_back（payload 含 rolled_back_to）。 目标 Revision 不属于该页面返回 404；并发语义与发布一致（行锁串行化）。

### Example

```ts
import {
  Configuration,
  HistoryApi,
} from '';
import type { RollbackPageRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new HistoryApi(config);

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // RollbackRequest
    rollbackRequest: ...,
  } satisfies RollbackPageRequest;

  try {
    const data = await api.rollbackPage(body);
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
| **rollbackRequest** | [RollbackRequest](RollbackRequest.md) |  | |

### Return type

[**RollbackResponse**](RollbackResponse.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 回滚产生的新 Revision（含 rolled_back_to） |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

