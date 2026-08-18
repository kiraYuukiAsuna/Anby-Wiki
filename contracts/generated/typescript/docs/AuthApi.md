# AuthApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**createCLIAuthCode**](AuthApi.md#createcliauthcodeoperation) | **POST** /api/v1/auth/cli/codes | 创建一次性 CLI 授权码 |
| [**exchangeCLIAuthCode**](AuthApi.md#exchangecliauthcodeoperation) | **POST** /api/v1/auth/cli/exchange | 兑换一次性授权码 |
| [**getSession**](AuthApi.md#getsession) | **GET** /api/v1/auth/session | 获取当前登录 Actor |
| [**listCLITokens**](AuthApi.md#listclitokens) | **GET** /api/v1/auth/cli/tokens | 列出当前账号的 CLI Token |
| [**login**](AuthApi.md#loginoperation) | **POST** /api/v1/auth/login | 使用用户名或邮箱和密码登录 |
| [**logout**](AuthApi.md#logout) | **POST** /api/v1/auth/logout | 吊销当前服务端 session 并清除 cookie |
| [**register**](AuthApi.md#registeroperation) | **POST** /api/v1/auth/register | 注册本地账号并建立会话 |
| [**revokeCLIToken**](AuthApi.md#revokeclitoken) | **DELETE** /api/v1/auth/cli/tokens/{id} | 撤销当前账号的指定 CLI Token |
| [**revokeCurrentCLIToken**](AuthApi.md#revokecurrentclitoken) | **DELETE** /api/v1/auth/cli/token | 撤销当前 Bearer Token |



## createCLIAuthCode

> CLIAuthCode createCLIAuthCode(createCLIAuthCodeRequest)

创建一次性 CLI 授权码

仅浏览器 Session 可调用。授权码十分钟内有效且只能兑换一次；响应中的明文码 不会再次返回，服务端只持久化 SHA-256。

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { CreateCLIAuthCodeOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AuthApi(config);

  const body = {
    // CreateCLIAuthCodeRequest
    createCLIAuthCodeRequest: ...,
  } satisfies CreateCLIAuthCodeOperationRequest;

  try {
    const data = await api.createCLIAuthCode(body);
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
| **createCLIAuthCodeRequest** | [CreateCLIAuthCodeRequest](CreateCLIAuthCodeRequest.md) |  | |

### Return type

[**CLIAuthCode**](CLIAuthCode.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 一次性授权码 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## exchangeCLIAuthCode

> CLIAuthExchangeResult exchangeCLIAuthCode(exchangeCLIAuthCodeRequest)

兑换一次性授权码

CLI 用短时授权码换取只显示一次的 Bearer Token。

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { ExchangeCLIAuthCodeOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  const body = {
    // ExchangeCLIAuthCodeRequest
    exchangeCLIAuthCodeRequest: ...,
  } satisfies ExchangeCLIAuthCodeOperationRequest;

  try {
    const data = await api.exchangeCLIAuthCode(body);
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
| **exchangeCLIAuthCodeRequest** | [ExchangeCLIAuthCodeRequest](ExchangeCLIAuthCodeRequest.md) |  | |

### Return type

[**CLIAuthExchangeResult**](CLIAuthExchangeResult.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | CLI Token；明文 token 只在本响应出现 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
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
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
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

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

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


## listCLITokens

> CLITokenList listCLITokens()

列出当前账号的 CLI Token

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { ListCLITokensRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AuthApi(config);

  try {
    const data = await api.listCLITokens();
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

[**CLITokenList**](CLITokenList.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 包含有效、过期和已撤销 Token 的审计目录 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
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


## revokeCLIToken

> revokeCLIToken(id)

撤销当前账号的指定 CLI Token

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { RevokeCLITokenRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AuthApi(config);

  const body = {
    // string
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies RevokeCLITokenRequest;

  try {
    const data = await api.revokeCLIToken(body);
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
| **id** | `string` |  | [Defaults to `undefined`] |

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
| **204** | Token 已撤销；重复撤销保持成功 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## revokeCurrentCLIToken

> revokeCurrentCLIToken()

撤销当前 Bearer Token

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { RevokeCurrentCLITokenRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
  });
  const api = new AuthApi(config);

  try {
    const data = await api.revokeCurrentCLIToken();
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

[cliBearer](../README.md#cliBearer)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | 当前 Token 已撤销 |  -  |
| **401** | 未认证 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

