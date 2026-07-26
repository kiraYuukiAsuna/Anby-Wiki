# AuthApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getSession**](AuthApi.md#getsession) | **GET** /api/v1/auth/session | 获取当前登录 Actor |
| [**login**](AuthApi.md#loginoperation) | **POST** /api/v1/auth/login | 使用用户名或邮箱和密码登录 |
| [**logout**](AuthApi.md#logout) | **POST** /api/v1/auth/logout | 吊销当前服务端 session 并清除 cookie |
| [**register**](AuthApi.md#registeroperation) | **POST** /api/v1/auth/register | 注册本地账号并建立会话 |



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


## login

> AuthResult login(loginRequest)

使用用户名或邮箱和密码登录

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { LoginOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  const body = {
    // LoginRequest
    loginRequest: ...,
  } satisfies LoginOperationRequest;

  try {
    const data = await api.login(body);
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
| **loginRequest** | [LoginRequest](LoginRequest.md) |  | |

### Return type

[**AuthResult**](AuthResult.md)

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
| **429** | 触发限流，需退避后重试 |  * Retry-After - 建议等待的秒数 <br>  * X-RateLimit-Limit - 当前窗口配额 <br>  * X-RateLimit-Remaining - 当前窗口剩余配额 <br>  |
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


## register

> AuthResult register(registerRequest)

注册本地账号并建立会话

用户名和邮箱均不区分大小写且全站唯一。首个本地账号获得管理员角色， 后续账号默认获得编辑者角色；注册默认关闭，部署方须通过 AUTH_REGISTRATION_ENABLED 显式开启。

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { RegisterOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  const body = {
    // RegisterRequest
    registerRequest: ...,
  } satisfies RegisterOperationRequest;

  try {
    const data = await api.register(body);
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
| **registerRequest** | [RegisterRequest](RegisterRequest.md) |  | |

### Return type

[**AuthResult**](AuthResult.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 注册成功，设置 HttpOnly session cookie |  -  |
| **400** | 请求格式错误 |  -  |
| **403** | 已认证但无权限 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **429** | 触发限流，需退避后重试 |  * Retry-After - 建议等待的秒数 <br>  * X-RateLimit-Limit - 当前窗口配额 <br>  * X-RateLimit-Remaining - 当前窗口剩余配额 <br>  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

