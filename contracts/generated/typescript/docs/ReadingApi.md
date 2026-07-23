# ReadingApi

All URIs are relative to *http://localhost:8000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getPageByID**](ReadingApi.md#getpagebyid) | **GET** /api/v1/pages/{id} | 按 ID 读取页面当前版本 |
| [**getPageByTitle**](ReadingApi.md#getpagebytitle) | **GET** /api/v1/pages/by-title | 按标题/别名读取页面当前版本 |



## getPageByID

> PageWithContent getPageByID(id)

按 ID 读取页面当前版本

阅读端点（匿名可读，无需登录）。按页面 ID 读取当前 Revision 与渲染 HTML， 响应结构与行为同 getPageByTitle（重定向同样跟随；软删除页返回 410 gone； 未发布过 content 为 null）。

### Example

```ts
import {
  Configuration,
  ReadingApi,
} from '';
import type { GetPageByIDRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ReadingApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetPageByIDRequest;

  try {
    const data = await api.getPageByID(body);
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

[**PageWithContent**](PageWithContent.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 页面元信息 + 当前 Revision 元信息 + AST + 渲染 HTML |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **410** | 资源曾存在但已删除（如软删除页面、重定向目标已删除） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getPageByTitle

> PageWithContent getPageByTitle(namespace, title)

按标题/别名读取页面当前版本

阅读端点（匿名可读，无需登录）。渲染直连 ContentSnapshot， M3 才引入 RenderedPage 投影与缓存。 标题经服务端规范化（NFC、大小写折叠、空白折叠）后先匹配活页面，再匹配 page_alias： - 活页面命中：200 PageWithContent，via_alias&#x3D;false； - 别名命中：200 PageWithContent，via_alias&#x3D;true 且 alias_title 回显请求标题； - 不存在：404 not_found。 页面是站内重定向源（page_redirect 有行）时跟随到落地页并返回其内容， 响应带 redirect: {from_page_id, from_title}；重定向环/超过最大跳数返回 422； 重定向目标已软删除返回 410 gone（资源曾存在、现已删除，语义区别于 404）。 页面已创建但未发布过时 200 且 content 为 null。

### Example

```ts
import {
  Configuration,
  ReadingApi,
} from '';
import type { GetPageByTitleRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ReadingApi();

  const body = {
    // string | 命名空间 key（如 main）
    namespace: main,
    // string | 页面标题（服务端规范化后匹配活页面或别名）
    title: Anby Demara,
  } satisfies GetPageByTitleRequest;

  try {
    const data = await api.getPageByTitle(body);
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
| **namespace** | `string` | 命名空间 key（如 main） | [Defaults to `undefined`] |
| **title** | `string` | 页面标题（服务端规范化后匹配活页面或别名） | [Defaults to `undefined`] |

### Return type

[**PageWithContent**](PageWithContent.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 页面元信息 + 当前 Revision 元信息 + AST + 渲染 HTML |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **410** | 资源曾存在但已删除（如软删除页面、重定向目标已删除） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

