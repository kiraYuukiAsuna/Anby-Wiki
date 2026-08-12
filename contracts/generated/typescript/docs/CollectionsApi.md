# CollectionsApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**createCollection**](CollectionsApi.md#createcollectionoperation) | **POST** /api/v1/collections | 创建 Manual、Rule 或 Dynamic Collection |
| [**getCollection**](CollectionsApi.md#getcollection) | **GET** /api/v1/collections/{id} | Collection 详情 |
| [**listCollectionMembers**](CollectionsApi.md#listcollectionmembers) | **GET** /api/v1/collections/{id}/members | Collection 物化成员 |
| [**listCollections**](CollectionsApi.md#listcollections) | **GET** /api/v1/collections | Collection 列表 |
| [**listPageCollections**](CollectionsApi.md#listpagecollections) | **GET** /api/v1/pages/{id}/collections | 页面所属 Collection |
| [**rebuildRuleCollection**](CollectionsApi.md#rebuildrulecollection) | **POST** /api/v1/collections/{id}/rebuild | 从 Entity/Claim 权威数据重建 Rule Collection 成员 |
| [**replaceManualCollectionMembers**](CollectionsApi.md#replacemanualcollectionmembers) | **PUT** /api/v1/collections/{id}/members | 原子替换 Manual Collection 的全部成员 |



## createCollection

> Collection createCollection(createCollectionRequest)

创建 Manual、Rule 或 Dynamic Collection

### Example

```ts
import {
  Configuration,
  CollectionsApi,
} from '';
import type { CreateCollectionOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new CollectionsApi(config);

  const body = {
    // CreateCollectionRequest
    createCollectionRequest: ...,
  } satisfies CreateCollectionOperationRequest;

  try {
    const data = await api.createCollection(body);
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
| **createCollectionRequest** | [CreateCollectionRequest](CreateCollectionRequest.md) |  | |

### Return type

[**Collection**](Collection.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | Collection |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getCollection

> Collection getCollection(id)

Collection 详情

匿名读取 Collection 定义；Manual 的 query 为 null，其余类型使用版本化安全查询。

### Example

```ts
import {
  Configuration,
  CollectionsApi,
} from '';
import type { GetCollectionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new CollectionsApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetCollectionRequest;

  try {
    const data = await api.getCollection(body);
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

[**Collection**](Collection.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Collection 详情 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listCollectionMembers

> CollectionMembershipListPage listCollectionMembers(id, cursor, pageSize)

Collection 物化成员

Manual/Rule 匿名读取物化 Membership；Dynamic 执行受限版本化查询。 两者均按 sort_key、member_type、target id 稳定游标分页，绝不扫描 AST。

### Example

```ts
import {
  Configuration,
  CollectionsApi,
} from '';
import type { ListCollectionMembersRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new CollectionsApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListCollectionMembersRequest;

  try {
    const data = await api.listCollectionMembers(body);
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

[**CollectionMembershipListPage**](CollectionMembershipListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 一页物化成员 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listCollections

> CollectionListPage listCollections(cursor, pageSize)

Collection 列表

匿名读取当前 Wiki 的 Manual/Rule/Dynamic Collection，按 title、id 稳定游标分页。列表只读权威定义，不执行查询。

### Example

```ts
import {
  Configuration,
  CollectionsApi,
} from '';
import type { ListCollectionsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new CollectionsApi();

  const body = {
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListCollectionsRequest;

  try {
    const data = await api.listCollections(body);
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

[**CollectionListPage**](CollectionListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 一页 Collection |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listPageCollections

> PageCollectionList listPageCollections(id)

页面所属 Collection

匿名反向读取页面直接所属、其主 Entity 所属以及 Dynamic 查询命中的 Collection。 membership_source 解释命中来源，不复制服务端 Collection 状态到客户端缓存。

### Example

```ts
import {
  Configuration,
  CollectionsApi,
} from '';
import type { ListPageCollectionsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new CollectionsApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies ListPageCollectionsRequest;

  try {
    const data = await api.listPageCollections(body);
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

[**PageCollectionList**](PageCollectionList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 页面所属 Collection |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## rebuildRuleCollection

> RebuildCollectionResult rebuildRuleCollection(id, rebuildCollectionRequest)

从 Entity/Claim 权威数据重建 Rule Collection 成员

### Example

```ts
import {
  Configuration,
  CollectionsApi,
} from '';
import type { RebuildRuleCollectionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new CollectionsApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // RebuildCollectionRequest
    rebuildCollectionRequest: ...,
  } satisfies RebuildRuleCollectionRequest;

  try {
    const data = await api.rebuildRuleCollection(body);
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
| **rebuildCollectionRequest** | [RebuildCollectionRequest](RebuildCollectionRequest.md) |  | |

### Return type

[**RebuildCollectionResult**](RebuildCollectionResult.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 重建结果 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## replaceManualCollectionMembers

> replaceManualCollectionMembers(id, replaceCollectionMembersRequest)

原子替换 Manual Collection 的全部成员

### Example

```ts
import {
  Configuration,
  CollectionsApi,
} from '';
import type { ReplaceManualCollectionMembersRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new CollectionsApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // ReplaceCollectionMembersRequest
    replaceCollectionMembersRequest: ...,
  } satisfies ReplaceManualCollectionMembersRequest;

  try {
    const data = await api.replaceManualCollectionMembers(body);
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
| **replaceCollectionMembersRequest** | [ReplaceCollectionMembersRequest](ReplaceCollectionMembersRequest.md) |  | |

### Return type

`void` (Empty response body)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | 成员已替换 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

