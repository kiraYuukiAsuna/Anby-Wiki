# AdminApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**deleteAdminUser**](AdminApi.md#deleteadminuser) | **DELETE** /api/v1/admin/users/{actor_id} | 管理员删除本地注册用户 |
| [**getAIConfig**](AdminApi.md#getaiconfig) | **GET** /api/v1/admin/ai-config | 读取站点 AI 运行时配置 |
| [**getSystemAccounts**](AdminApi.md#getsystemaccounts) | **GET** /api/v1/system/accounts | 系统管理员读取账号安全概况与系统安全审计 |
| [**getSystemMail**](AdminApi.md#getsystemmail) | **GET** /api/v1/system/mail | 系统管理员读取全站 SMTP 配置，密码永不返回 |
| [**grantAdminUserRole**](AdminApi.md#grantadminuserrole) | **PUT** /api/v1/admin/users/{actor_id}/roles/{role_key} | 管理员授予用户角色 |
| [**listAdminUsers**](AdminApi.md#listadminusers) | **GET** /api/v1/admin/users | 管理员列出本地用户与角色 |
| [**revokeAdminUserRole**](AdminApi.md#revokeadminuserrole) | **DELETE** /api/v1/admin/users/{actor_id}/roles/{role_key} | 管理员撤销用户角色 |
| [**setSystemAdministrator**](AdminApi.md#setsystemadministratoroperation) | **PUT** /api/v1/system/administrators | 重新认证后授予或撤销独立的系统管理员权限 |
| [**testAIConfig**](AdminApi.md#testaiconfig) | **POST** /api/v1/admin/ai-config/test | 使用已保存配置执行最小结构化模型请求 |
| [**testSystemMail**](AdminApi.md#testsystemmailoperation) | **POST** /api/v1/system/mail/test | 系统管理员向指定收件邮箱发送测试邮件 |
| [**updateAIConfig**](AdminApi.md#updateaiconfigoperation) | **PUT** /api/v1/admin/ai-config | 保存站点 AI 运行时配置 |
| [**updateSystemMail**](AdminApi.md#updatesystemmailoperation) | **PUT** /api/v1/system/mail | 系统管理员重新认证后保存 SMTP 配置 |



## deleteAdminUser

> AdminUserDeletionResult deleteAdminUser(actorId)

管理员删除本地注册用户

删除本地登录账号并使其 Session 与 CLI Token 失效；历史内容保留 disabled actor 作为审计主体。不能删除当前账号或最后一个 active admin。

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { DeleteAdminUserRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AdminApi(config);

  const body = {
    // string
    actorId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies DeleteAdminUserRequest;

  try {
    const data = await api.deleteAdminUser(body);
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
| **actorId** | `string` |  | [Defaults to `undefined`] |

### Return type

[**AdminUserDeletionResult**](AdminUserDeletionResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 删除结果 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


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
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
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

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

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


## getSystemAccounts

> SystemAccounts getSystemAccounts(search, accountsPage, eventsPage, pageSize)

系统管理员读取账号安全概况与系统安全审计

账号与安全审计独立分页，默认各返回 10 条，并返回匹配总数与实际页码； 超出结果范围的页码收敛到最后一页。账号搜索覆盖全部账号，不限于当前页。 账号按系统管理员优先、创建时间及 Actor ID 排序；审计按时间及 ID 倒序。 mfa_enabled 表示当前操作账号的 MFA 状态，不受账号搜索或分页影响。 不返回密码、密钥、令牌或私人 Wiki 内容。

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { GetSystemAccountsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AdminApi(config);

  const body = {
    // string | 按用户名或邮箱搜索，忽略大小写并按字面子串匹配；最大 254 字节。 (optional)
    search: search_example,
    // number (optional)
    accountsPage: 56,
    // number (optional)
    eventsPage: 56,
    // number | 两类列表各自的每页条数。 (optional)
    pageSize: 56,
  } satisfies GetSystemAccountsRequest;

  try {
    const data = await api.getSystemAccounts(body);
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
| **search** | `string` | 按用户名或邮箱搜索，忽略大小写并按字面子串匹配；最大 254 字节。 | [Optional] [Defaults to `undefined`] |
| **accountsPage** | `number` |  | [Optional] [Defaults to `1`] |
| **eventsPage** | `number` |  | [Optional] [Defaults to `1`] |
| **pageSize** | `number` | 两类列表各自的每页条数。 | [Optional] [Defaults to `10`] |

### Return type

[**SystemAccounts**](SystemAccounts.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 安全概况 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getSystemMail

> SystemMailConfig getSystemMail()

系统管理员读取全站 SMTP 配置，密码永不返回

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { GetSystemMailRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AdminApi(config);

  try {
    const data = await api.getSystemMail();
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

[**SystemMailConfig**](SystemMailConfig.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 邮件配置 |  -  |
| **401** | 未认证 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## grantAdminUserRole

> AdminRoleMutationResult grantAdminUserRole(actorId, roleKey)

管理员授予用户角色

幂等操作；角色已存在时返回 changed&#x3D;false。

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { GrantAdminUserRoleRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AdminApi(config);

  const body = {
    // string
    actorId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // 'editor' | 'reviewer' | 'applier' | 'admin'
    roleKey: roleKey_example,
  } satisfies GrantAdminUserRoleRequest;

  try {
    const data = await api.grantAdminUserRole(body);
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
| **actorId** | `string` |  | [Defaults to `undefined`] |
| **roleKey** | `editor`, `reviewer`, `applier`, `admin` |  | [Defaults to `undefined`] [Enum: editor, reviewer, applier, admin] |

### Return type

[**AdminRoleMutationResult**](AdminRoleMutationResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 授权后的用户角色快照 |  -  |
| **401** | 未认证 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listAdminUsers

> AdminUserList listAdminUsers(search, includeDisabled)

管理员列出本地用户与角色

仅 admin 可用；角色变更不会写入 session，下一次授权检查即时生效。

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { ListAdminUsersRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AdminApi(config);

  const body = {
    // string | 按用户名、邮箱、显示名或 Actor ID 过滤。 (optional)
    search: search_example,
    // boolean (optional)
    includeDisabled: true,
  } satisfies ListAdminUsersRequest;

  try {
    const data = await api.listAdminUsers(body);
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
| **search** | `string` | 按用户名、邮箱、显示名或 Actor ID 过滤。 | [Optional] [Defaults to `undefined`] |
| **includeDisabled** | `boolean` |  | [Optional] [Defaults to `false`] |

### Return type

[**AdminUserList**](AdminUserList.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 本地用户与当前 Wiki 角色 |  -  |
| **401** | 未认证 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## revokeAdminUserRole

> AdminRoleMutationResult revokeAdminUserRole(actorId, roleKey)

管理员撤销用户角色

幂等操作；角色不存在时返回 changed&#x3D;false。最后一个 active human admin 不能被撤销。

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { RevokeAdminUserRoleRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AdminApi(config);

  const body = {
    // string
    actorId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // 'editor' | 'reviewer' | 'applier' | 'admin'
    roleKey: roleKey_example,
  } satisfies RevokeAdminUserRoleRequest;

  try {
    const data = await api.revokeAdminUserRole(body);
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
| **actorId** | `string` |  | [Defaults to `undefined`] |
| **roleKey** | `editor`, `reviewer`, `applier`, `admin` |  | [Defaults to `undefined`] [Enum: editor, reviewer, applier, admin] |

### Return type

[**AdminRoleMutationResult**](AdminRoleMutationResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 撤销后的用户角色快照 |  -  |
| **401** | 未认证 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## setSystemAdministrator

> setSystemAdministrator(setSystemAdministratorRequest)

重新认证后授予或撤销独立的系统管理员权限

初始 admin 账号的系统管理员权限不可撤销；最后一个有效系统管理员不能被撤销；Wiki 管理员不自动获得系统权限。

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { SetSystemAdministratorOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AdminApi(config);

  const body = {
    // SetSystemAdministratorRequest
    setSystemAdministratorRequest: ...,
  } satisfies SetSystemAdministratorOperationRequest;

  try {
    const data = await api.setSystemAdministrator(body);
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
| **setSystemAdministratorRequest** | [SetSystemAdministratorRequest](SetSystemAdministratorRequest.md) |  | |

### Return type

`void` (Empty response body)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | 权限已更新 |  -  |
| **400** | 请求格式错误 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **429** | 触发限流，需退避后重试 |  * Retry-After - 建议等待的秒数 <br>  * X-RateLimit-Limit - 当前窗口配额 <br>  * X-RateLimit-Remaining - 当前窗口剩余配额 <br>  |

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
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
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

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

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


## testSystemMail

> testSystemMail(testSystemMailRequest)

系统管理员向指定收件邮箱发送测试邮件

使用已保存的 SMTP 配置；需要有效的系统管理员浏览器会话，无需再次输入密码或二次验证码。

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { TestSystemMailOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AdminApi(config);

  const body = {
    // TestSystemMailRequest
    testSystemMailRequest: ...,
  } satisfies TestSystemMailOperationRequest;

  try {
    const data = await api.testSystemMail(body);
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
| **testSystemMailRequest** | [TestSystemMailRequest](TestSystemMailRequest.md) |  | |

### Return type

`void` (Empty response body)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | 邮件已提交 SMTP |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **429** | 触发限流，需退避后重试 |  * Retry-After - 建议等待的秒数 <br>  * X-RateLimit-Limit - 当前窗口配额 <br>  * X-RateLimit-Remaining - 当前窗口剩余配额 <br>  |
| **503** | 服务端内部错误 |  -  |

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
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
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

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

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


## updateSystemMail

> SystemMailConfig updateSystemMail(updateSystemMailRequest)

系统管理员重新认证后保存 SMTP 配置

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { UpdateSystemMailOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AdminApi(config);

  const body = {
    // UpdateSystemMailRequest
    updateSystemMailRequest: ...,
  } satisfies UpdateSystemMailOperationRequest;

  try {
    const data = await api.updateSystemMail(body);
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
| **updateSystemMailRequest** | [UpdateSystemMailRequest](UpdateSystemMailRequest.md) |  | |

### Return type

[**SystemMailConfig**](SystemMailConfig.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 已保存 |  -  |
| **400** | 请求格式错误 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **429** | 触发限流，需退避后重试 |  * Retry-After - 建议等待的秒数 <br>  * X-RateLimit-Limit - 当前窗口配额 <br>  * X-RateLimit-Remaining - 当前窗口剩余配额 <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

