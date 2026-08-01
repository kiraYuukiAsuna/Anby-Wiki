# PagesApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**createPage**](PagesApi.md#createpageoperation) | **POST** /api/v1/pages | 创建页面 |
| [**createPageRedirect**](PagesApi.md#createpageredirectoperation) | **POST** /api/v1/pages/{id}/redirect | 创建或更新页面重定向 |
| [**deleteBlockRedirect**](PagesApi.md#deleteblockredirect) | **DELETE** /api/v1/pages/{id}/block-redirects/{block_id} | 删除错误的章节迁移映射（审计历史保留） |
| [**deletePageRedirect**](PagesApi.md#deletepageredirect) | **DELETE** /api/v1/pages/{id}/redirect | 删除页面当前重定向 |
| [**getPageRedirect**](PagesApi.md#getpageredirect) | **GET** /api/v1/pages/{id}/redirect | 读取页面当前重定向 |
| [**listBlockRedirects**](PagesApi.md#listblockredirects) | **GET** /api/v1/pages/{id}/block-redirects | 列出页面的稳定章节迁移映射 |
| [**publishRevision**](PagesApi.md#publishrevisionoperation) | **POST** /api/v1/pages/{id}/revisions | 发布 Revision |
| [**renamePage**](PagesApi.md#renamepageoperation) | **POST** /api/v1/pages/{id}/rename | 页面改名 |
| [**upsertBlockRedirect**](PagesApi.md#upsertblockredirectoperation) | **PUT** /api/v1/pages/{id}/block-redirects/{block_id} | 创建或更新稳定章节迁移映射 |



## createPage

> Page createPage(createPageRequest)

创建页面

在默认站点（wiki 固定为种子里 site_key&#x3D;\&#39;default\&#39; 的站点）创建页面。 标题经服务端规范化（NFC、大小写折叠、空白折叠），与同 wiki+namespace 的 活页面或别名冲突时返回 409。 Actor 身份由服务端 session cookie 解析；客户端不能声明 Actor。

### Example

```ts
import {
  Configuration,
  PagesApi,
} from '';
import type { CreatePageOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new PagesApi(config);

  const body = {
    // CreatePageRequest
    createPageRequest: ...,
  } satisfies CreatePageOperationRequest;

  try {
    const data = await api.createPage(body);
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
| **createPageRequest** | [CreatePageRequest](CreatePageRequest.md) |  | |

### Return type

[**Page**](Page.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 页面已创建 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createPageRedirect

> PageRedirect createPageRedirect(id, createPageRedirectRequest)

创建或更新页面重定向

支持同 Wiki Page、Page 稳定章节、尚未解析的命名空间/标题和外部 Wiki URL。 服务端验证判别联合、章节身份与完整内部链路，拒绝自重定向、循环和已删除目标。

### Example

```ts
import {
  Configuration,
  PagesApi,
} from '';
import type { CreatePageRedirectOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new PagesApi(config);

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // CreatePageRedirectRequest
    createPageRedirectRequest: ...,
  } satisfies CreatePageRedirectOperationRequest;

  try {
    const data = await api.createPageRedirect(body);
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
| **createPageRedirectRequest** | [CreatePageRedirectRequest](CreatePageRedirectRequest.md) |  | |

### Return type

[**PageRedirect**](PageRedirect.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 已写入重定向 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **410** | 资源曾存在但已删除（如软删除页面、重定向目标已删除） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## deleteBlockRedirect

> deleteBlockRedirect(id, blockId)

删除错误的章节迁移映射（审计历史保留）

### Example

```ts
import {
  Configuration,
  PagesApi,
} from '';
import type { DeleteBlockRedirectRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new PagesApi(config);

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string
    blockId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies DeleteBlockRedirectRequest;

  try {
    const data = await api.deleteBlockRedirect(body);
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

`void` (Empty response body)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | 已删除 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## deletePageRedirect

> deletePageRedirect(id)

删除页面当前重定向

### Example

```ts
import {
  Configuration,
  PagesApi,
} from '';
import type { DeletePageRedirectRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new PagesApi(config);

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies DeletePageRedirectRequest;

  try {
    const data = await api.deletePageRedirect(body);
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

`void` (Empty response body)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | 重定向已删除 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getPageRedirect

> PageRedirect getPageRedirect(id)

读取页面当前重定向

### Example

```ts
import {
  Configuration,
  PagesApi,
} from '';
import type { GetPageRedirectRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new PagesApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetPageRedirectRequest;

  try {
    const data = await api.getPageRedirect(body);
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

[**PageRedirect**](PageRedirect.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前权威重定向 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listBlockRedirects

> BlockRedirectList listBlockRedirects(id)

列出页面的稳定章节迁移映射

### Example

```ts
import {
  Configuration,
  PagesApi,
} from '';
import type { ListBlockRedirectsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new PagesApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies ListBlockRedirectsRequest;

  try {
    const data = await api.listBlockRedirects(body);
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

[**BlockRedirectList**](BlockRedirectList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | BlockRedirect 列表 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## publishRevision

> Revision publishRevision(id, publishRevisionRequest)

发布 Revision

原子发布一个 Revision：校验 AST（Typed Block AST v1，schema_version&#x3D;1）后， 在单事务内写入 ContentSnapshot / Revision / 页面当前指针 / AuditEvent / OutboxEvent。 乐观锁：expected_revision_id 必须等于页面当前 Revision（首发布不传， 要求页面尚无 Revision）；不一致返回 409 stale_revision。 content_hash 与 size_bytes 由服务端对 canonical AST 计算，客户端提供的值不被信任。

### Example

```ts
import {
  Configuration,
  PagesApi,
} from '';
import type { PublishRevisionOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new PagesApi(config);

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // PublishRevisionRequest
    publishRevisionRequest: ...,
  } satisfies PublishRevisionOperationRequest;

  try {
    const data = await api.publishRevision(body);
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
| **publishRevisionRequest** | [PublishRevisionRequest](PublishRevisionRequest.md) |  | |

### Return type

[**Revision**](Revision.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 发布成功的 Revision |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（陈旧基线 stale_revision） |  * X-Request-ID -  <br>  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## renamePage

> Page renamePage(id, renamePageRequest)

页面改名

更新页面标题，旧标题写入 page_alias（Page ID 不变）。 新标题被同 wiki+namespace 的活页面或其他页面的别名占用时返回 409。

### Example

```ts
import {
  Configuration,
  PagesApi,
} from '';
import type { RenamePageOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new PagesApi(config);

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // RenamePageRequest
    renamePageRequest: ...,
  } satisfies RenamePageOperationRequest;

  try {
    const data = await api.renamePage(body);
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
| **renamePageRequest** | [RenamePageRequest](RenamePageRequest.md) |  | |

### Return type

[**Page**](Page.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 改名后的页面 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## upsertBlockRedirect

> BlockRedirect upsertBlockRedirect(id, blockId, upsertBlockRedirectRequest)

创建或更新稳定章节迁移映射

目标会解析并折叠到当前有效 Heading Block；服务端拒绝环和悬空目标。

### Example

```ts
import {
  Configuration,
  PagesApi,
} from '';
import type { UpsertBlockRedirectOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new PagesApi(config);

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string
    blockId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // UpsertBlockRedirectRequest
    upsertBlockRedirectRequest: ...,
  } satisfies UpsertBlockRedirectOperationRequest;

  try {
    const data = await api.upsertBlockRedirect(body);
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
| **upsertBlockRedirectRequest** | [UpsertBlockRedirectRequest](UpsertBlockRedirectRequest.md) |  | |

### Return type

[**BlockRedirect**](BlockRedirect.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前 BlockRedirect |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

