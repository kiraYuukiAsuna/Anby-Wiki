# DatasetsApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**createDataset**](DatasetsApi.md#createdatasetoperation) | **POST** /api/v1/datasets | 创建带 JSON Schema 的 Dataset |
| [**createDatasetRecord**](DatasetsApi.md#createdatasetrecord) | **POST** /api/v1/datasets/{id}/records | 创建经 Dataset Schema 校验的记录 |
| [**createDatasetView**](DatasetsApi.md#createdatasetviewoperation) | **POST** /api/v1/datasets/{id}/views | 创建筛选、排序与分组视图 |
| [**getDataset**](DatasetsApi.md#getdataset) | **GET** /api/v1/datasets/{id} | 读取 Dataset 定义 |
| [**getDatasetView**](DatasetsApi.md#getdatasetview) | **GET** /api/v1/dataset-views/{id} | 读取保存的 DatasetView |
| [**listDatasetRecords**](DatasetsApi.md#listdatasetrecords) | **GET** /api/v1/datasets/{id}/records | 按稳定默认顺序列出 DatasetRecord |
| [**listDatasetViews**](DatasetsApi.md#listdatasetviews) | **GET** /api/v1/datasets/{id}/views | 列出 Dataset 的保存视图 |
| [**listDatasets**](DatasetsApi.md#listdatasets) | **GET** /api/v1/datasets | 列出站点 Dataset |
| [**queryDatasetView**](DatasetsApi.md#querydatasetview) | **GET** /api/v1/dataset-views/{id}/records | 执行保存视图的筛选、排序与分组 |
| [**updateDatasetRecord**](DatasetsApi.md#updatedatasetrecord) | **PUT** /api/v1/dataset-records/{id} | 更新记录并重新执行 Dataset Schema 校验 |



## createDataset

> Dataset createDataset(createDatasetRequest)

创建带 JSON Schema 的 Dataset

### Example

```ts
import {
  Configuration,
  DatasetsApi,
} from '';
import type { CreateDatasetOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new DatasetsApi(config);

  const body = {
    // CreateDatasetRequest
    createDatasetRequest: ...,
  } satisfies CreateDatasetOperationRequest;

  try {
    const data = await api.createDataset(body);
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
| **createDatasetRequest** | [CreateDatasetRequest](CreateDatasetRequest.md) |  | |

### Return type

[**Dataset**](Dataset.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | Dataset |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createDatasetRecord

> DatasetRecord createDatasetRecord(id, writeDatasetRecordRequest)

创建经 Dataset Schema 校验的记录

### Example

```ts
import {
  Configuration,
  DatasetsApi,
} from '';
import type { CreateDatasetRecordRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new DatasetsApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // WriteDatasetRecordRequest
    writeDatasetRecordRequest: ...,
  } satisfies CreateDatasetRecordRequest;

  try {
    const data = await api.createDatasetRecord(body);
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
| **writeDatasetRecordRequest** | [WriteDatasetRecordRequest](WriteDatasetRecordRequest.md) |  | |

### Return type

[**DatasetRecord**](DatasetRecord.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | DatasetRecord |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createDatasetView

> DatasetView createDatasetView(id, createDatasetViewRequest)

创建筛选、排序与分组视图

### Example

```ts
import {
  Configuration,
  DatasetsApi,
} from '';
import type { CreateDatasetViewOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new DatasetsApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // CreateDatasetViewRequest
    createDatasetViewRequest: ...,
  } satisfies CreateDatasetViewOperationRequest;

  try {
    const data = await api.createDatasetView(body);
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
| **createDatasetViewRequest** | [CreateDatasetViewRequest](CreateDatasetViewRequest.md) |  | |

### Return type

[**DatasetView**](DatasetView.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | DatasetView |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getDataset

> Dataset getDataset(id)

读取 Dataset 定义

### Example

```ts
import {
  Configuration,
  DatasetsApi,
} from '';
import type { GetDatasetRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new DatasetsApi();

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetDatasetRequest;

  try {
    const data = await api.getDataset(body);
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

[**Dataset**](Dataset.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Dataset |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getDatasetView

> DatasetView getDatasetView(id)

读取保存的 DatasetView

### Example

```ts
import {
  Configuration,
  DatasetsApi,
} from '';
import type { GetDatasetViewRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new DatasetsApi();

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetDatasetViewRequest;

  try {
    const data = await api.getDatasetView(body);
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

[**DatasetView**](DatasetView.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | DatasetView |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listDatasetRecords

> DatasetRecordPage listDatasetRecords(id, cursor, pageSize)

按稳定默认顺序列出 DatasetRecord

### Example

```ts
import {
  Configuration,
  DatasetsApi,
} from '';
import type { ListDatasetRecordsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new DatasetsApi();

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListDatasetRecordsRequest;

  try {
    const data = await api.listDatasetRecords(body);
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
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**DatasetRecordPage**](DatasetRecordPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | DatasetRecord 分页结果 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listDatasetViews

> DatasetViewList listDatasetViews(id)

列出 Dataset 的保存视图

### Example

```ts
import {
  Configuration,
  DatasetsApi,
} from '';
import type { ListDatasetViewsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new DatasetsApi();

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies ListDatasetViewsRequest;

  try {
    const data = await api.listDatasetViews(body);
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

[**DatasetViewList**](DatasetViewList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | DatasetView 列表 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listDatasets

> DatasetListPage listDatasets(cursor, pageSize)

列出站点 Dataset

### Example

```ts
import {
  Configuration,
  DatasetsApi,
} from '';
import type { ListDatasetsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new DatasetsApi();

  const body = {
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListDatasetsRequest;

  try {
    const data = await api.listDatasets(body);
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

[**DatasetListPage**](DatasetListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Dataset 游标分页结果 |  -  |
| **400** | 请求格式错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## queryDatasetView

> DatasetRecordPage queryDatasetView(id, cursor, pageSize)

执行保存视图的筛选、排序与分组

### Example

```ts
import {
  Configuration,
  DatasetsApi,
} from '';
import type { QueryDatasetViewRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new DatasetsApi();

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies QueryDatasetViewRequest;

  try {
    const data = await api.queryDatasetView(body);
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
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**DatasetRecordPage**](DatasetRecordPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | DatasetView 查询结果 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## updateDatasetRecord

> DatasetRecord updateDatasetRecord(id, writeDatasetRecordRequest)

更新记录并重新执行 Dataset Schema 校验

### Example

```ts
import {
  Configuration,
  DatasetsApi,
} from '';
import type { UpdateDatasetRecordRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new DatasetsApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // WriteDatasetRecordRequest
    writeDatasetRecordRequest: ...,
  } satisfies UpdateDatasetRecordRequest;

  try {
    const data = await api.updateDatasetRecord(body);
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
| **writeDatasetRecordRequest** | [WriteDatasetRecordRequest](WriteDatasetRecordRequest.md) |  | |

### Return type

[**DatasetRecord**](DatasetRecord.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | DatasetRecord |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

