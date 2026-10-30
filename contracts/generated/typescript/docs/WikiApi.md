# WikiApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**acceptPageShareLink**](WikiApi.md#acceptpagesharelinkoperation) | **POST** /api/v1/page-share-links/accept | 领取页面分享链接 |
| [**addWikiMember**](WikiApi.md#addwikimemberoperation) | **POST** /api/v1/wiki/members | 添加协作者并授予角色 |
| [**createPageShareLink**](WikiApi.md#createpagesharelinkoperation) | **POST** /api/v1/pages/{id}/share-links | 为当前页面创建分享链接 |
| [**grantWikiMemberRole**](WikiApi.md#grantwikimemberrole) | **PUT** /api/v1/wiki/members/{actor_id}/roles/{role_key} | 授予当前 Wiki 成员角色 |
| [**listPageShares**](WikiApi.md#listpageshares) | **GET** /api/v1/pages/{id}/shares | 列出当前页面的单文档分享对象 |
| [**listWikiMembers**](WikiApi.md#listwikimembers) | **GET** /api/v1/wiki/members | 列出当前 Wiki 成员与角色 |
| [**removePageShare**](WikiApi.md#removepageshare) | **DELETE** /api/v1/pages/{id}/shares/{actor_id} | 取消当前页面对指定账号的分享 |
| [**removeWikiMember**](WikiApi.md#removewikimember) | **DELETE** /api/v1/wiki/members/{actor_id} | 移除当前 Wiki 协作者 |
| [**revokeWikiMemberRole**](WikiApi.md#revokewikimemberrole) | **DELETE** /api/v1/wiki/members/{actor_id}/roles/{role_key} | 撤销当前 Wiki 成员角色 |
| [**upsertPageShare**](WikiApi.md#upsertpageshareoperation) | **PUT** /api/v1/pages/{id}/shares | 分享当前页面给本地账号 |



## acceptPageShareLink

> PageShareLinkAcceptanceResult acceptPageShareLink(acceptPageShareLinkRequest)

领取页面分享链接

登录用户访问分享链接后领取页面级 viewer/editor 权限；一次性链接只允许首个登录用户领取。token 放在请求体，避免进入 API access log path。

### Example

```ts
import {
  Configuration,
  WikiApi,
} from '';
import type { AcceptPageShareLinkOperationRequest } from '';

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
    // AcceptPageShareLinkRequest
    acceptPageShareLinkRequest: ...,
  } satisfies AcceptPageShareLinkOperationRequest;

  try {
    const data = await api.acceptPageShareLink(body);
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
| **acceptPageShareLinkRequest** | [AcceptPageShareLinkRequest](AcceptPageShareLinkRequest.md) |  | |

### Return type

[**PageShareLinkAcceptanceResult**](PageShareLinkAcceptanceResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 页面分享权限领取结果 |  -  |
| **401** | 未认证 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


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
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createPageShareLink

> PageShareLinkCreationResult createPageShareLink(id, createPageShareLinkRequest)

为当前页面创建分享链接

仅当前 Wiki admin 可用；返回的 token 只在本次响应出现，服务端仅保存 hash。

### Example

```ts
import {
  Configuration,
  WikiApi,
} from '';
import type { CreatePageShareLinkOperationRequest } from '';

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
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // CreatePageShareLinkRequest
    createPageShareLinkRequest: ...,
  } satisfies CreatePageShareLinkOperationRequest;

  try {
    const data = await api.createPageShareLink(body);
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
| **createPageShareLinkRequest** | [CreatePageShareLinkRequest](CreatePageShareLinkRequest.md) |  | |

### Return type

[**PageShareLinkCreationResult**](PageShareLinkCreationResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 页面分享链接创建结果 |  -  |
| **401** | 未认证 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
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
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listPageShares

> PageShareList listPageShares(id)

列出当前页面的单文档分享对象

仅当前 Wiki admin 可用；页面分享不写入 actor_role，不授予 Wiki 级可见性。

### Example

```ts
import {
  Configuration,
  WikiApi,
} from '';
import type { ListPageSharesRequest } from '';

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
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies ListPageSharesRequest;

  try {
    const data = await api.listPageShares(body);
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

[**PageShareList**](PageShareList.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 页面分享对象列表 |  -  |
| **401** | 未认证 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **404** | 资源不存在 |  -  |

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
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## removePageShare

> PageShareRemovalResult removePageShare(id, actorId)

取消当前页面对指定账号的分享

删除 page_share 记录；不会删除账号，也不会影响其个人 Wiki 或 Wiki 成员角色。

### Example

```ts
import {
  Configuration,
  WikiApi,
} from '';
import type { RemovePageShareRequest } from '';

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
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string
    actorId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies RemovePageShareRequest;

  try {
    const data = await api.removePageShare(body);
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
| **actorId** | `string` |  | [Defaults to `undefined`] |

### Return type

[**PageShareRemovalResult**](PageShareRemovalResult.md)

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
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **404** | 资源不存在 |  -  |

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
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
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
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## upsertPageShare

> PageShareMutationResult upsertPageShare(id, upsertPageShareRequest)

分享当前页面给本地账号

按用户名、邮箱或 Actor ID 查找 active 本地账号；幂等写入页面级 viewer/editor。

### Example

```ts
import {
  Configuration,
  WikiApi,
} from '';
import type { UpsertPageShareOperationRequest } from '';

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
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // UpsertPageShareRequest
    upsertPageShareRequest: ...,
  } satisfies UpsertPageShareOperationRequest;

  try {
    const data = await api.upsertPageShare(body);
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
| **upsertPageShareRequest** | [UpsertPageShareRequest](UpsertPageShareRequest.md) |  | |

### Return type

[**PageShareMutationResult**](PageShareMutationResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 写入后的页面分享快照 |  -  |
| **401** | 未认证 |  -  |
| **403** | 无操作权限或浏览器写请求来源不受信任。 非安全方法统一使用 Go CrossOriginProtection 校验浏览器来源；优先检查 Sec-Fetch-Site，缺失时比较 Origin 与 Host。无来源头的 CLI/服务端调用 继续执行原有认证和授权。  |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

