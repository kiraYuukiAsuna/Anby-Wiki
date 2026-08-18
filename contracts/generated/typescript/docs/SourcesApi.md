# SourcesApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**createCitation**](SourcesApi.md#createcitationoperation) | **POST** /api/v1/citations | 创建不可变 Citation |
| [**createSource**](SourcesApi.md#createsource) | **POST** /api/v1/sources | 登记逻辑来源 |
| [**getSource**](SourcesApi.md#getsource) | **GET** /api/v1/sources/{id} | 读取来源与规范化外部资源 |
| [**listSourceChunks**](SourcesApi.md#listsourcechunks) | **GET** /api/v1/source-versions/{id}/chunks | 分页列出不可变来源版本的可定位分片 |
| [**listSourceVersions**](SourcesApi.md#listsourceversions) | **GET** /api/v1/sources/{id}/versions | 分页列出来源的不可变版本 |
| [**listSources**](SourcesApi.md#listsources) | **GET** /api/v1/sources | 分页浏览逻辑来源目录 |



## createCitation

> CitationRecord createCitation(createCitationRequest)

创建不可变 Citation

引文若绑定 SourceChunk，必须是该分片文本的真实子串。

### Example

```ts
import {
  Configuration,
  SourcesApi,
} from '';
import type { CreateCitationOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new SourcesApi(config);

  const body = {
    // CreateCitationRequest
    createCitationRequest: ...,
  } satisfies CreateCitationOperationRequest;

  try {
    const data = await api.createCitation(body);
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
| **createCitationRequest** | [CreateCitationRequest](CreateCitationRequest.md) |  | |

### Return type

[**CitationRecord**](CitationRecord.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 已创建 Citation |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createSource

> EvidenceSource createSource(createEvidenceSourceRequest)

登记逻辑来源

URL 会经领域服务规范化并幂等关联 ExternalResource。

### Example

```ts
import {
  Configuration,
  SourcesApi,
} from '';
import type { CreateSourceRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new SourcesApi(config);

  const body = {
    // CreateEvidenceSourceRequest
    createEvidenceSourceRequest: ...,
  } satisfies CreateSourceRequest;

  try {
    const data = await api.createSource(body);
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
| **createEvidenceSourceRequest** | [CreateEvidenceSourceRequest](CreateEvidenceSourceRequest.md) |  | |

### Return type

[**EvidenceSource**](EvidenceSource.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 已创建来源 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getSource

> EvidenceSourceDetail getSource(id)

读取来源与规范化外部资源

### Example

```ts
import {
  Configuration,
  SourcesApi,
} from '';
import type { GetSourceRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new SourcesApi();

  const body = {
    // string
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetSourceRequest;

  try {
    const data = await api.getSource(body);
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
| **id** | `string` |  | [Defaults to `undefined`] |

### Return type

[**EvidenceSourceDetail**](EvidenceSourceDetail.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 来源详情 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listSourceChunks

> EvidenceSourceChunkListPage listSourceChunks(id, cursor, pageSize)

分页列出不可变来源版本的可定位分片

SourceChunk 正文可能包含来源全文片段，不写入日志。

### Example

```ts
import {
  Configuration,
  SourcesApi,
} from '';
import type { ListSourceChunksRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new SourcesApi();

  const body = {
    // string
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListSourceChunksRequest;

  try {
    const data = await api.listSourceChunks(body);
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
| **id** | `string` |  | [Defaults to `undefined`] |
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**EvidenceSourceChunkListPage**](EvidenceSourceChunkListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 来源分片目录 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listSourceVersions

> EvidenceSourceVersionListPage listSourceVersions(id, cursor, pageSize)

分页列出来源的不可变版本

### Example

```ts
import {
  Configuration,
  SourcesApi,
} from '';
import type { ListSourceVersionsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new SourcesApi();

  const body = {
    // string
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListSourceVersionsRequest;

  try {
    const data = await api.listSourceVersions(body);
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
| **id** | `string` |  | [Defaults to `undefined`] |
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**EvidenceSourceVersionListPage**](EvidenceSourceVersionListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 来源版本目录 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listSources

> EvidenceSourceListPage listSources(cursor, pageSize, sourceType, q)

分页浏览逻辑来源目录

只查询来源元数据，不读取 SourceChunk 正文。

### Example

```ts
import {
  Configuration,
  SourcesApi,
} from '';
import type { ListSourcesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new SourcesApi();

  const body = {
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
    // 'webpage' | 'pdf' | 'book' | 'image' | 'video' | 'api' | 'database' (optional)
    sourceType: sourceType_example,
    // string | 标题、作者或发布者的元数据搜索，最长 200 字符。 (optional)
    q: q_example,
  } satisfies ListSourcesRequest;

  try {
    const data = await api.listSources(body);
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
| **sourceType** | `webpage`, `pdf`, `book`, `image`, `video`, `api`, `database` |  | [Optional] [Defaults to `undefined`] [Enum: webpage, pdf, book, image, video, api, database] |
| **q** | `string` | 标题、作者或发布者的元数据搜索，最长 200 字符。 | [Optional] [Defaults to `undefined`] |

### Return type

[**EvidenceSourceListPage**](EvidenceSourceListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 来源目录 |  -  |
| **400** | 请求格式错误 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

