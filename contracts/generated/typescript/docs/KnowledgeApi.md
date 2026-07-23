# KnowledgeApi

All URIs are relative to *http://localhost:8000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getCitation**](KnowledgeApi.md#getcitation) | **GET** /api/v1/citations/{id} | Citation 详情与证据定位链 |
| [**getClaim**](KnowledgeApi.md#getclaim) | **GET** /api/v1/claims/{id} | Claim 详情 |
| [**getEntity**](KnowledgeApi.md#getentity) | **GET** /api/v1/entities/{id} | Entity 详情 |
| [**mergeEntity**](KnowledgeApi.md#mergeentityoperation) | **POST** /api/v1/entities/{id}/merge | 合并重复 Entity |



## getCitation

> CitationDetail getCitation(id)

Citation 详情与证据定位链

匿名读取不可变 Citation，并沿 SourceVersion 定位到 Source、可选 Chunk 与外部资源 URL。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { GetCitationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetCitationRequest;

  try {
    const data = await api.getCitation(body);
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

### Return type

[**CitationDetail**](CitationDetail.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Citation 只读详情及完整定位上下文 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getClaim

> ClaimDetail getClaim(id)

Claim 详情

匿名读取 Claim 的谓词、结构化值、业务/验证状态、取代链及 Citation 绑定。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { GetClaimRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetClaimRequest;

  try {
    const data = await api.getClaim(body);
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

### Return type

[**ClaimDetail**](ClaimDetail.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Claim 只读详情 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getEntity

> EntityDetail getEntity(id)

Entity 详情

匿名读取 Entity 的稳定身份、类型、状态、标签与别名；本端点不提供写操作。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { GetEntityRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetEntityRequest;

  try {
    const data = await api.getEntity(body);
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

### Return type

[**EntityDetail**](EntityDetail.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Entity 只读详情 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## mergeEntity

> EntityMergeResult mergeEntity(id, mergeEntityRequest)

合并重复 Entity

仅站点管理员或 system Actor 可触发。合并、审计与 entity.merged Outbox 在同一事务提交；引用修复 Proposal 由 Worker 异步幂等生成。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { MergeEntityOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // MergeEntityRequest
    mergeEntityRequest: ...,
  } satisfies MergeEntityOperationRequest;

  try {
    const data = await api.mergeEntity(body);
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
| **mergeEntityRequest** | [MergeEntityRequest](MergeEntityRequest.md) |  | |

### Return type

[**EntityMergeResult**](EntityMergeResult.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 合并完成；同一 source/target 的重复请求返回既有结果 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

