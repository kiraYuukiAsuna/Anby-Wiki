# PagesApi

All URIs are relative to *http://localhost:8000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**createPage**](PagesApi.md#createpageoperation) | **POST** /api/v1/pages | 创建页面 |
| [**publishRevision**](PagesApi.md#publishrevisionoperation) | **POST** /api/v1/pages/{id}/revisions | 发布 Revision |
| [**renamePage**](PagesApi.md#renamepageoperation) | **POST** /api/v1/pages/{id}/rename | 页面改名 |



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

