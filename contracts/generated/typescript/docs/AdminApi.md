# AdminApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**deleteAdminUser**](AdminApi.md#deleteadminuser) | **DELETE** /api/v1/admin/users/{actor_id} | 管理员删除本地注册用户 |
| [**getAIConfig**](AdminApi.md#getaiconfig) | **GET** /api/v1/admin/ai-config | 读取站点 AI 运行时配置 |
| [**grantAdminUserRole**](AdminApi.md#grantadminuserrole) | **PUT** /api/v1/admin/users/{actor_id}/roles/{role_key} | 管理员授予用户角色 |
| [**listAdminUsers**](AdminApi.md#listadminusers) | **GET** /api/v1/admin/users | 管理员列出本地用户与角色 |
| [**revokeAdminUserRole**](AdminApi.md#revokeadminuserrole) | **DELETE** /api/v1/admin/users/{actor_id}/roles/{role_key} | 管理员撤销用户角色 |
| [**testAIConfig**](AdminApi.md#testaiconfig) | **POST** /api/v1/admin/ai-config/test | 使用已保存配置执行最小结构化模型请求 |
| [**updateAIConfig**](AdminApi.md#updateaiconfigoperation) | **PUT** /api/v1/admin/ai-config | 保存站点 AI 运行时配置 |



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
| **403** | 已认证但无权限 |  -  |
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
| **403** | 已认证但无权限 |  -  |
| **500** | 服务端内部错误 |  -  |

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
| **403** | 已认证但无权限 |  -  |
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
| **403** | 已认证但无权限 |  -  |
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
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

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
| **403** | 已认证但无权限 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

