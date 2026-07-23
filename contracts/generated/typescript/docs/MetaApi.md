# MetaApi

All URIs are relative to *http://localhost:8000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getHealthz**](MetaApi.md#gethealthz) | **GET** /healthz | 存活探针 |
| [**getReadyz**](MetaApi.md#getreadyz) | **GET** /readyz | 就绪探针 |



## getHealthz

> HealthResponse getHealthz()

存活探针

恒 200，仅表示进程存活，不检查任何依赖。

### Example

```ts
import {
  Configuration,
  MetaApi,
} from '';
import type { GetHealthzRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new MetaApi();

  try {
    const data = await api.getHealthz();
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters

This endpoint does not need any parameter.

### Return type

[**HealthResponse**](HealthResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 进程存活 |  * X-Request-ID -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getReadyz

> ReadyResponse getReadyz()

就绪探针

逐项检查依赖（postgres、redis 等）；任一不可达或未配置返回 503。

### Example

```ts
import {
  Configuration,
  MetaApi,
} from '';
import type { GetReadyzRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new MetaApi();

  try {
    const data = await api.getReadyz();
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters

This endpoint does not need any parameter.

### Return type

[**ReadyResponse**](ReadyResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 全部依赖就绪 |  * X-Request-ID -  <br>  |
| **503** | 存在不可达或未配置的依赖 |  * X-Request-ID -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

