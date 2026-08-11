# AdminApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getAIConfig**](AdminApi.md#getaiconfig) | **GET** /api/v1/admin/ai-config | 读取站点 AI 运行时配置 |
| [**testAIConfig**](AdminApi.md#testaiconfig) | **POST** /api/v1/admin/ai-config/test | 使用已保存配置执行最小结构化模型请求 |
| [**updateAIConfig**](AdminApi.md#updateaiconfigoperation) | **PUT** /api/v1/admin/ai-config | 保存站点 AI 运行时配置 |



## getAIConfig

> AIConfig getAIConfig()

读取站点 AI 运行时配置

仅管理员可见；API Key 永不返回，只暴露是否已配置。

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { GetAIConfigRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AdminApi(config);

  try {
    const data = await api.getAIConfig();
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

[**AIConfig**](AIConfig.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 脱敏后的当前配置；未保存时返回安全默认值 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## testAIConfig

> AIConfigTestResult testAIConfig()

使用已保存配置执行最小结构化模型请求

不接受临时密钥或 Prompt；仅返回供应商、模型与端到端耗时。

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { TestAIConfigRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AdminApi(config);

  try {
    const data = await api.testAIConfig();
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

[**AIConfigTestResult**](AIConfigTestResult.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Semantic Kernel 与模型供应商连通且结构化输出有效 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **502** | 服务端内部错误 |  -  |
| **504** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## updateAIConfig

> AIConfig updateAIConfig(updateAIConfigRequest)

保存站点 AI 运行时配置

配置与审计事件原子写入。首次保存必须提供 api_key；后续省略或传空字符串 表示保留现有密钥。密钥使用部署主密钥加密后持久化。

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { UpdateAIConfigOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AdminApi(config);

  const body = {
    // UpdateAIConfigRequest
    updateAIConfigRequest: ...,
  } satisfies UpdateAIConfigOperationRequest;

  try {
    const data = await api.updateAIConfig(body);
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
| **updateAIConfigRequest** | [UpdateAIConfigRequest](UpdateAIConfigRequest.md) |  | |

### Return type

[**AIConfig**](AIConfig.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 脱敏后的已保存配置 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

