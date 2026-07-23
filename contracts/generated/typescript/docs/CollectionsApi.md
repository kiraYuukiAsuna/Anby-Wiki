# CollectionsApi

All URIs are relative to *http://localhost:8000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getCollection**](CollectionsApi.md#getcollection) | **GET** /api/v1/collections/{id} | Collection 详情 |
| [**listCollectionMembers**](CollectionsApi.md#listcollectionmembers) | **GET** /api/v1/collections/{id}/members | Collection 物化成员 |
| [**listCollections**](CollectionsApi.md#listcollections) | **GET** /api/v1/collections | Collection 列表 |



## getCollection

> Collection getCollection(id)

Collection 详情

匿名读取 Collection 定义；Manual 的 query 为 null，Rule 的 query 为 v1 判别联合。

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

匿名读取已物化 Membership，按 sort_key、member_type、target id 稳定游标分页。 本端点不扫描 AST，也不在请求时执行 Rule。

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

匿名读取当前 Wiki 的 Manual/Rule Collection，按 title、id 稳定游标分页。 该端点只读取权威 Collection 定义，不实时执行 Rule。

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

