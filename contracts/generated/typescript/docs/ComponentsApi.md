# ComponentsApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**createWikiComponent**](ComponentsApi.md#createwikicomponentoperation) | **POST** /api/v1/components | 创建稳定组件注册项 |
| [**createWikiComponentVersion**](ComponentsApi.md#createwikicomponentversion) | **POST** /api/v1/components/{id}/versions | 创建可编辑的组件草稿版本 |
| [**deprecateWikiComponentVersion**](ComponentsApi.md#deprecatewikicomponentversion) | **POST** /api/v1/components/{id}/versions/{version}/deprecate | 废弃已发布组件版本 |
| [**getWikiComponent**](ComponentsApi.md#getwikicomponent) | **GET** /api/v1/components/{id} | 读取组件注册项 |
| [**listWikiComponentVersions**](ComponentsApi.md#listwikicomponentversions) | **GET** /api/v1/components/{id}/versions | 列出组件版本 |
| [**listWikiComponents**](ComponentsApi.md#listwikicomponents) | **GET** /api/v1/components | 列出组件注册项 |
| [**previewWikiComponentVersion**](ComponentsApi.md#previewwikicomponentversion) | **POST** /api/v1/components/{id}/versions/{version}/preview | 按 Props Schema 校验并用可信渲染器预览组件 |
| [**publishWikiComponentVersion**](ComponentsApi.md#publishwikicomponentversion) | **POST** /api/v1/components/{id}/versions/{version}/publish | 发布并冻结组件版本 |
| [**updateWikiComponentVersion**](ComponentsApi.md#updatewikicomponentversion) | **PUT** /api/v1/components/{id}/versions/{version} | 更新尚未发布的组件草稿 |



## createWikiComponent

> WikiComponent createWikiComponent(createWikiComponentRequest)

创建稳定组件注册项

### Example

```ts
import {
  Configuration,
  ComponentsApi,
} from '';
import type { CreateWikiComponentOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new ComponentsApi(config);

  const body = {
    // CreateWikiComponentRequest
    createWikiComponentRequest: ...,
  } satisfies CreateWikiComponentOperationRequest;

  try {
    const data = await api.createWikiComponent(body);
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
| **createWikiComponentRequest** | [CreateWikiComponentRequest](CreateWikiComponentRequest.md) |  | |

### Return type

[**WikiComponent**](WikiComponent.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 组件 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createWikiComponentVersion

> WikiComponentVersion createWikiComponentVersion(id, writeWikiComponentVersionRequest)

创建可编辑的组件草稿版本

### Example

```ts
import {
  Configuration,
  ComponentsApi,
} from '';
import type { CreateWikiComponentVersionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new ComponentsApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // WriteWikiComponentVersionRequest
    writeWikiComponentVersionRequest: ...,
  } satisfies CreateWikiComponentVersionRequest;

  try {
    const data = await api.createWikiComponentVersion(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |
| **writeWikiComponentVersionRequest** | [WriteWikiComponentVersionRequest](WriteWikiComponentVersionRequest.md) |  | |

### Return type

[**WikiComponentVersion**](WikiComponentVersion.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 草稿版本 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## deprecateWikiComponentVersion

> WikiComponentVersion deprecateWikiComponentVersion(id, version)

废弃已发布组件版本

### Example

```ts
import {
  Configuration,
  ComponentsApi,
} from '';
import type { DeprecateWikiComponentVersionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new ComponentsApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // number | 正整数组件版本
    version: 56,
  } satisfies DeprecateWikiComponentVersionRequest;

  try {
    const data = await api.deprecateWikiComponentVersion(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |
| **version** | `number` | 正整数组件版本 | [Defaults to `undefined`] |

### Return type

[**WikiComponentVersion**](WikiComponentVersion.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 已废弃版本 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getWikiComponent

> WikiComponent getWikiComponent(id)

读取组件注册项

### Example

```ts
import {
  Configuration,
  ComponentsApi,
} from '';
import type { GetWikiComponentRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ComponentsApi();

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetWikiComponentRequest;

  try {
    const data = await api.getWikiComponent(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**WikiComponent**](WikiComponent.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 组件 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listWikiComponentVersions

> WikiComponentVersionList listWikiComponentVersions(id)

列出组件版本

### Example

```ts
import {
  Configuration,
  ComponentsApi,
} from '';
import type { ListWikiComponentVersionsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ComponentsApi();

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies ListWikiComponentVersionsRequest;

  try {
    const data = await api.listWikiComponentVersions(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**WikiComponentVersionList**](WikiComponentVersionList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 组件版本列表 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listWikiComponents

> WikiComponentListPage listWikiComponents(cursor, pageSize)

列出组件注册项

### Example

```ts
import {
  Configuration,
  ComponentsApi,
} from '';
import type { ListWikiComponentsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ComponentsApi();

  const body = {
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListWikiComponentsRequest;

  try {
    const data = await api.listWikiComponents(body);
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
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**WikiComponentListPage**](WikiComponentListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 组件游标分页结果 |  -  |
| **400** | 请求格式错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## previewWikiComponentVersion

> PreviewWikiComponentResult previewWikiComponentVersion(id, version, previewWikiComponentRequest)

按 Props Schema 校验并用可信渲染器预览组件

### Example

```ts
import {
  Configuration,
  ComponentsApi,
} from '';
import type { PreviewWikiComponentVersionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new ComponentsApi();

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // number | 正整数组件版本
    version: 56,
    // PreviewWikiComponentRequest
    previewWikiComponentRequest: ...,
  } satisfies PreviewWikiComponentVersionRequest;

  try {
    const data = await api.previewWikiComponentVersion(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |
| **version** | `number` | 正整数组件版本 | [Defaults to `undefined`] |
| **previewWikiComponentRequest** | [PreviewWikiComponentRequest](PreviewWikiComponentRequest.md) |  | |

### Return type

[**PreviewWikiComponentResult**](PreviewWikiComponentResult.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 可信服务端渲染 HTML |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## publishWikiComponentVersion

> WikiComponentVersion publishWikiComponentVersion(id, version)

发布并冻结组件版本

### Example

```ts
import {
  Configuration,
  ComponentsApi,
} from '';
import type { PublishWikiComponentVersionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new ComponentsApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // number | 正整数组件版本
    version: 56,
  } satisfies PublishWikiComponentVersionRequest;

  try {
    const data = await api.publishWikiComponentVersion(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |
| **version** | `number` | 正整数组件版本 | [Defaults to `undefined`] |

### Return type

[**WikiComponentVersion**](WikiComponentVersion.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 已发布版本 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## updateWikiComponentVersion

> WikiComponentVersion updateWikiComponentVersion(id, version, writeWikiComponentVersionRequest)

更新尚未发布的组件草稿

### Example

```ts
import {
  Configuration,
  ComponentsApi,
} from '';
import type { UpdateWikiComponentVersionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new ComponentsApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // number | 正整数组件版本
    version: 56,
    // WriteWikiComponentVersionRequest
    writeWikiComponentVersionRequest: ...,
  } satisfies UpdateWikiComponentVersionRequest;

  try {
    const data = await api.updateWikiComponentVersion(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |
| **version** | `number` | 正整数组件版本 | [Defaults to `undefined`] |
| **writeWikiComponentVersionRequest** | [WriteWikiComponentVersionRequest](WriteWikiComponentVersionRequest.md) |  | |

### Return type

[**WikiComponentVersion**](WikiComponentVersion.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 更新后的草稿 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

