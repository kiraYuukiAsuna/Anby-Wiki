# AuthApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**devLogin**](AuthApi.md#devloginoperation) | **POST** /api/v1/auth/dev-login | 用引导令牌换取服务端 session（早期阶段占位登录） |
| [**getSession**](AuthApi.md#getsession) | **GET** /api/v1/auth/session | 获取当前登录 Actor |
| [**logout**](AuthApi.md#logout) | **POST** /api/v1/auth/logout | 吊销当前服务端 session 并清除 cookie |



## devLogin

> DevLoginResult devLogin(devLoginRequest)

用引导令牌换取服务端 session（早期阶段占位登录）

早期阶段占位登录：以共享引导令牌换取 HttpOnly session cookie。 该端点不验证调用者真实身份，仅用于封闭的早期部署， 公网暴露前必须替换为真实身份提供方。 未启用时返回 404。

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { DevLoginOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  const body = {
    // DevLoginRequest
    devLoginRequest: ...,
  } satisfies DevLoginOperationRequest;

  try {
    const data = await api.devLogin(body);
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
| **devLoginRequest** | [DevLoginRequest](DevLoginRequest.md) |  | |

### Return type

[**DevLoginResult**](DevLoginResult.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 登录成功，设置 HttpOnly session cookie |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **404** | 资源不存在 |  -  |
| **429** | 触发限流，需退避后重试 |  * Retry-After - 建议等待的秒数 <br>  * X-RateLimit-Limit - 当前窗口配额 <br>  * X-RateLimit-Remaining - 当前窗口剩余配额 <br>  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getSession

> AuthSession getSession()

获取当前登录 Actor

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { GetSessionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AuthApi(config);

  try {
    const data = await api.getSession();
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

[**AuthSession**](AuthSession.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前登录会话 |  -  |
| **401** | 未认证 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## logout

> logout()

吊销当前服务端 session 并清除 cookie

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { LogoutRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AuthApi(config);

  try {
    const data = await api.logout();
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

`void` (Empty response body)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | 已退出；无活动 session 时同样成功 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

