# ImportsApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**cancelImportJob**](ImportsApi.md#cancelimportjob) | **POST** /api/v1/import-jobs/{id}/cancel | 取消排队中或运行中的导入任务 |
| [**createImportJob**](ImportsApi.md#createimportjoboperation) | **POST** /api/v1/import-jobs | 幂等创建来源导入任务 |
| [**createImportUploadJob**](ImportsApi.md#createimportuploadjob) | **POST** /api/v1/import-jobs/uploads | 校验并暂存用户文件，幂等创建来源导入任务 |
| [**getImportJob**](ImportsApi.md#getimportjob) | **GET** /api/v1/import-jobs/{id} | 读取导入任务和各次运行阶段进度 |
| [**listImportJobs**](ImportsApi.md#listimportjobs) | **GET** /api/v1/import-jobs | 分页列出当前账号的导入任务 |
| [**retryImportJob**](ImportsApi.md#retryimportjob) | **POST** /api/v1/import-jobs/{id}/retry | 将失败或已取消任务重新排队 |



## cancelImportJob

> ImportJobDetail cancelImportJob(id)

取消排队中或运行中的导入任务

### Example

```ts
import {
  Configuration,
  ImportsApi,
} from '';
import type { CancelImportJobRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new ImportsApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies CancelImportJobRequest;

  try {
    const data = await api.cancelImportJob(body);
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

[**ImportJobDetail**](ImportJobDetail.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 已取消的任务详情 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createImportJob

> ImportJob createImportJob(idempotencyKey, createImportJobRequest)

幂等创建来源导入任务

### Example

```ts
import {
  Configuration,
  ImportsApi,
} from '';
import type { CreateImportJobOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new ImportsApi(config);

  const body = {
    // string | 变更类请求的幂等键（客户端生成的 UUID）。 服务端对相同 Actor + 幂等键的重复请求返回首次处理结果，不重复执行。
    idempotencyKey: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // CreateImportJobRequest
    createImportJobRequest: ...,
  } satisfies CreateImportJobOperationRequest;

  try {
    const data = await api.createImportJob(body);
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
| **idempotencyKey** | `string` | 变更类请求的幂等键（客户端生成的 UUID）。 服务端对相同 Actor + 幂等键的重复请求返回首次处理结果，不重复执行。  | [Defaults to `undefined`] |
| **createImportJobRequest** | [CreateImportJobRequest](CreateImportJobRequest.md) |  | |

### Return type

[**ImportJob**](ImportJob.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 已排队的导入任务 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createImportUploadJob

> ImportJob createImportUploadJob(idempotencyKey, file, title, instructions, routeMode)

校验并暂存用户文件，幂等创建来源导入任务

接受最大 10 MiB 的 HTML、纯文本、JSON、CSV、PDF、PNG 或 JPEG。API 在排队前 执行 MIME/magic、大小与恶意签名检查，并经 Evidence 领域服务写入私有对象存储； Worker 获取后会再次校验内容哈希与全部安全门禁。图片与无文本层 PDF 进入受限 OCR；JSON API 快照与 CSV/JSON 数据库导出保留为可定位 SourceChunk。原始内容 不写入 ImportJob config 或诊断日志。

### Example

```ts
import {
  Configuration,
  ImportsApi,
} from '';
import type { CreateImportUploadJobRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new ImportsApi(config);

  const body = {
    // string | 变更类请求的幂等键（客户端生成的 UUID）。 服务端对相同 Actor + 幂等键的重复请求返回首次处理结果，不重复执行。
    idempotencyKey: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // Blob
    file: BINARY_DATA_HERE,
    // string (optional)
    title: title_example,
    // string (optional)
    instructions: instructions_example,
    // string (optional)
    routeMode: routeMode_example,
  } satisfies CreateImportUploadJobRequest;

  try {
    const data = await api.createImportUploadJob(body);
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
| **idempotencyKey** | `string` | 变更类请求的幂等键（客户端生成的 UUID）。 服务端对相同 Actor + 幂等键的重复请求返回首次处理结果，不重复执行。  | [Defaults to `undefined`] |
| **file** | `Blob` |  | [Defaults to `undefined`] |
| **title** | `string` |  | [Optional] [Defaults to `undefined`] |
| **instructions** | `string` |  | [Optional] [Defaults to `undefined`] |
| **routeMode** | `auto`, `force_create` |  | [Optional] [Defaults to `&#39;auto&#39;`] [Enum: auto, force_create] |

### Return type

[**ImportJob**](ImportJob.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `multipart/form-data`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 已暂存来源并排队的导入任务 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **413** | 请求格式错误 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |
| **503** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getImportJob

> ImportJobDetail getImportJob(id)

读取导入任务和各次运行阶段进度

### Example

```ts
import {
  Configuration,
  ImportsApi,
} from '';
import type { GetImportJobRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new ImportsApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetImportJobRequest;

  try {
    const data = await api.getImportJob(body);
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

[**ImportJobDetail**](ImportJobDetail.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 导入任务详情 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listImportJobs

> ImportJobListPage listImportJobs(cursor, pageSize, status)

分页列出当前账号的导入任务

按创建时间倒序返回，刷新或重新登录后仍可恢复完整导入队列。

### Example

```ts
import {
  Configuration,
  ImportsApi,
} from '';
import type { ListImportJobsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new ImportsApi(config);

  const body = {
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
    // 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled' (optional)
    status: status_example,
  } satisfies ListImportJobsRequest;

  try {
    const data = await api.listImportJobs(body);
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
| **status** | `queued`, `running`, `succeeded`, `failed`, `cancelled` |  | [Optional] [Defaults to `undefined`] [Enum: queued, running, succeeded, failed, cancelled] |

### Return type

[**ImportJobListPage**](ImportJobListPage.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前账号的导入任务 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## retryImportJob

> ImportJobDetail retryImportJob(id)

将失败或已取消任务重新排队

### Example

```ts
import {
  Configuration,
  ImportsApi,
} from '';
import type { RetryImportJobRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new ImportsApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies RetryImportJobRequest;

  try {
    const data = await api.retryImportJob(body);
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

[**ImportJobDetail**](ImportJobDetail.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 已重新排队的任务详情 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

