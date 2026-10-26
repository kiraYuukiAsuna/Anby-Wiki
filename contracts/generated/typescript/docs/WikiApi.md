# WikiApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**addWikiMember**](WikiApi.md#addwikimemberoperation) | **POST** /api/v1/wiki/members | 添加协作者并授予角色 |
| [**grantWikiMemberRole**](WikiApi.md#grantwikimemberrole) | **PUT** /api/v1/wiki/members/{actor_id}/roles/{role_key} | 授予当前 Wiki 成员角色 |
| [**listWikiMembers**](WikiApi.md#listwikimembers) | **GET** /api/v1/wiki/members | 列出当前 Wiki 成员与角色 |
| [**removeWikiMember**](WikiApi.md#removewikimember) | **DELETE** /api/v1/wiki/members/{actor_id} | 移除当前 Wiki 协作者 |
| [**revokeWikiMemberRole**](WikiApi.md#revokewikimemberrole) | **DELETE** /api/v1/wiki/members/{actor_id}/roles/{role_key} | 撤销当前 Wiki 成员角色 |



## addWikiMember

> WikiMemberMutationResult addWikiMember(addWikiMemberRequest)

添加协作者并授予角色

按用户名、邮箱或 Actor ID 查找 active 本地账号；幂等添加角色。

### Example

```ts
import {
  Configuration,
  WikiApi,
} from '';
import type { AddWikiMemberOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new WikiApi(config);

  const body = {
    // AddWikiMemberRequest
    addWikiMemberRequest: ...,
  } satisfies AddWikiMemberOperationRequest;

  try {
    const data = await api.addWikiMember(body);
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
| **addWikiMemberRequest** | [AddWikiMemberRequest](AddWikiMemberRequest.md) |  | |

### Return type

[**WikiMemberMutationResult**](WikiMemberMutationResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 添加后的成员角色快照 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## grantWikiMemberRole

> WikiMemberMutationResult grantWikiMemberRole(actorId, roleKey)

授予当前 Wiki 成员角色

幂等操作；成员必须已在当前 Wiki 中。

### Example

```ts
import {
  Configuration,
  WikiApi,
} from '';
import type { GrantWikiMemberRoleRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new WikiApi(config);

  const body = {
    // string
    actorId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // 'editor' | 'reviewer' | 'applier' | 'admin'
    roleKey: roleKey_example,
  } satisfies GrantWikiMemberRoleRequest;

  try {
    const data = await api.grantWikiMemberRole(body);
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

[**WikiMemberMutationResult**](WikiMemberMutationResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 授权后的成员角色快照 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listWikiMembers

> WikiMemberList listWikiMembers(search, includeDisabled)

列出当前 Wiki 成员与角色

仅 admin 可用；成员关系以 actor_role(actor_id,wiki_id,role_id) 为准。

### Example

```ts
import {
  Configuration,
  WikiApi,
} from '';
import type { ListWikiMembersRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new WikiApi(config);

  const body = {
    // string | 按用户名、邮箱、显示名或 Actor ID 过滤。 (optional)
    search: search_example,
    // boolean (optional)
    includeDisabled: true,
  } satisfies ListWikiMembersRequest;

  try {
    const data = await api.listWikiMembers(body);
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

[**WikiMemberList**](WikiMemberList.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前 Wiki 成员列表 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## removeWikiMember

> WikiMemberRemovalResult removeWikiMember(actorId)

移除当前 Wiki 协作者

仅撤销该 Actor 在当前 Wiki 的角色，不删除本地账号或其个人 Wiki。

### Example

```ts
import {
  Configuration,
  WikiApi,
} from '';
import type { RemoveWikiMemberRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new WikiApi(config);

  const body = {
    // string
    actorId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies RemoveWikiMemberRequest;

  try {
    const data = await api.removeWikiMember(body);
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

[**WikiMemberRemovalResult**](WikiMemberRemovalResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 移除结果 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## revokeWikiMemberRole

> WikiMemberMutationResult revokeWikiMemberRole(actorId, roleKey)

撤销当前 Wiki 成员角色

成员至少保留一个角色；如需完全移出成员列表，请调用移除协作者接口。

### Example

```ts
import {
  Configuration,
  WikiApi,
} from '';
import type { RevokeWikiMemberRoleRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new WikiApi(config);

  const body = {
    // string
    actorId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // 'editor' | 'reviewer' | 'applier' | 'admin'
    roleKey: roleKey_example,
  } satisfies RevokeWikiMemberRoleRequest;

  try {
    const data = await api.revokeWikiMemberRole(body);
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

[**WikiMemberMutationResult**](WikiMemberMutationResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 撤销后的成员角色快照 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

