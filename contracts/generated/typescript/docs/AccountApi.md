# AccountApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getAccountAIConfig**](AccountApi.md#getaccountaiconfig) | **GET** /api/v1/account/ai-config | 读取当前账号的模型配置 |
| [**getAccountProfile**](AccountApi.md#getaccountprofile) | **GET** /api/v1/account/profile | 读取当前账号资料 |
| [**testAccountAIConfig**](AccountApi.md#testaccountaiconfig) | **POST** /api/v1/account/ai-config/test | 测试当前账号已保存的模型配置 |
| [**updateAccountAIConfig**](AccountApi.md#updateaccountaiconfig) | **PUT** /api/v1/account/ai-config | 保存当前账号的模型配置 |
| [**updateAccountProfile**](AccountApi.md#updateaccountprofileoperation) | **PUT** /api/v1/account/profile | 更新当前账号资料 |



## getAccountAIConfig

> AIConfig getAccountAIConfig()

读取当前账号的模型配置

仅读取当前账号拥有的私人 Wiki 配置，选择协作 Wiki 不改变范围。API Key 永不返回，只暴露是否已配置。

### Example

```ts
import {
  Configuration,
  AccountApi,
} from '';
import type { GetAccountAIConfigRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AccountApi(config);

  try {
    const data = await api.getAccountAIConfig();
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
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getAccountProfile

> AccountProfile getAccountProfile()

读取当前账号资料

### Example

```ts
import {
  Configuration,
  AccountApi,
} from '';
import type { GetAccountProfileRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AccountApi(config);

  try {
    const data = await api.getAccountProfile();
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

[**AccountProfile**](AccountProfile.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前账号资料 |  -  |
| **401** | 未认证 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## testAccountAIConfig

> AIConfigTestResult testAccountAIConfig()

测试当前账号已保存的模型配置

使用当前账号拥有的私人 Wiki 配置，选择协作 Wiki 不改变范围。不接受临时密钥或 Prompt；仅返回供应商、模型与端到端耗时。

### Example

```ts
import {
  Configuration,
  AccountApi,
} from '';
import type { TestAccountAIConfigRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AccountApi(config);

  try {
    const data = await api.testAccountAIConfig();
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
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **502** | 服务端内部错误 |  -  |
| **504** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## updateAccountAIConfig

> AIConfig updateAccountAIConfig(updateAIConfigRequest)

保存当前账号的模型配置

模型配置写入当前账号拥有的私人 Wiki，选择协作 Wiki 不改变范围。首次保存必须提供 api_key； 后续省略或传空字符串表示保留现有密钥。密钥使用部署主密钥加密后持久化。

### Example

```ts
import {
  Configuration,
  AccountApi,
} from '';
import type { UpdateAccountAIConfigRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AccountApi(config);

  const body = {
    // UpdateAIConfigRequest
    updateAIConfigRequest: ...,
  } satisfies UpdateAccountAIConfigRequest;

  try {
    const data = await api.updateAccountAIConfig(body);
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
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## updateAccountProfile

> AccountProfile updateAccountProfile(updateAccountProfileRequest)

更新当前账号资料

### Example

```ts
import {
  Configuration,
  AccountApi,
} from '';
import type { UpdateAccountProfileOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AccountApi(config);

  const body = {
    // UpdateAccountProfileRequest
    updateAccountProfileRequest: ...,
  } satisfies UpdateAccountProfileOperationRequest;

  try {
    const data = await api.updateAccountProfile(body);
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
| **updateAccountProfileRequest** | [UpdateAccountProfileRequest](UpdateAccountProfileRequest.md) |  | |

### Return type

[**AccountProfile**](AccountProfile.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 更新后的账号资料 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

