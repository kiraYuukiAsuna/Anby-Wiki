# AuthApi

All URIs are relative to *http://localhost:8000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**callback**](AuthApi.md#callback) | **GET** /api/v1/auth/callback | 消费 OIDC callback 并建立服务端 session |
| [**getSession**](AuthApi.md#getsession) | **GET** /api/v1/auth/session | 获取当前登录 Actor |
| [**login**](AuthApi.md#login) | **GET** /api/v1/auth/login | 开始 OIDC Authorization Code + PKCE 登录 |
| [**logout**](AuthApi.md#logout) | **POST** /api/v1/auth/logout | 吊销当前服务端 session 并清除 cookie |



## callback

> callback(code, state, error)

消费 OIDC callback 并建立服务端 session

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { CallbackRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  const body = {
    // string (optional)
    code: code_example,
    // string (optional)
    state: state_example,
    // string (optional)
    error: error_example,
  } satisfies CallbackRequest;

  try {
    const data = await api.callback(body);
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
| **code** | `string` |  | [Optional] [Defaults to `undefined`] |
| **state** | `string` |  | [Optional] [Defaults to `undefined`] |
| **error** | `string` |  | [Optional] [Defaults to `undefined`] |

### Return type

`void` (Empty response body)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **302** | 登录成功，设置 HttpOnly session cookie 并重定向回站内 |  * Location -  <br>  |
| **401** | 未认证 |  -  |
| **500** | 服务端内部错误 |  -  |
| **503** | 服务端内部错误 |  -  |

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


## login

> login()

开始 OIDC Authorization Code + PKCE 登录

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { LoginRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  try {
    const data = await api.login();
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

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **302** | 重定向至身份提供方 |  * Location -  <br>  |
| **500** | 服务端内部错误 |  -  |
| **503** | 服务端内部错误 |  -  |

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

