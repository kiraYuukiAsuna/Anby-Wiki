# KnowledgeApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**addEntityAlias**](KnowledgeApi.md#addentityaliasoperation) | **POST** /api/v1/entities/{id}/aliases | 新增 Entity 别名 |
| [**addEntityLabel**](KnowledgeApi.md#addentitylabeloperation) | **POST** /api/v1/entities/{id}/labels | 新增 Entity 标签 |
| [**createEntityFederationLink**](KnowledgeApi.md#createentityfederationlinkoperation) | **POST** /api/v1/entities/{id}/federation-links | 为 Entity 创建跨 Wiki 映射 |
| [**getCitation**](KnowledgeApi.md#getcitation) | **GET** /api/v1/citations/{id} | Citation 详情与证据定位链 |
| [**getClaim**](KnowledgeApi.md#getclaim) | **GET** /api/v1/claims/{id} | Claim 详情 |
| [**getEntity**](KnowledgeApi.md#getentity) | **GET** /api/v1/entities/{id} | Entity 详情 |
| [**getEntityGraph**](KnowledgeApi.md#getentitygraph) | **GET** /api/v1/entities/{id}/graph | 查询 Entity 关系图 |
| [**getEntityMerge**](KnowledgeApi.md#getentitymerge) | **GET** /api/v1/entities/{id}/merge | 读取 Entity 的最近合并记录 |
| [**listEntities**](KnowledgeApi.md#listentities) | **GET** /api/v1/entities | 浏览与检索 Entity 目录 |
| [**listEntityFederationLinks**](KnowledgeApi.md#listentityfederationlinks) | **GET** /api/v1/entities/{id}/federation-links | 列出一个 Entity 的跨 Wiki 映射 |
| [**listFactConsistencyIssues**](KnowledgeApi.md#listfactconsistencyissues) | **GET** /api/v1/fact-consistency-issues | 列出自动事实一致性问题 |
| [**listFederatedWikis**](KnowledgeApi.md#listfederatedwikis) | **GET** /api/v1/federated-wikis | 列出跨 Wiki 联邦目录 |
| [**listFederationLinks**](KnowledgeApi.md#listfederationlinks) | **GET** /api/v1/federation-links | 浏览跨 Wiki Entity 映射 |
| [**listPageEntityBindings**](KnowledgeApi.md#listpageentitybindings) | **GET** /api/v1/pages/{id}/entity-bindings | 列出 Page 的 Entity 绑定 |
| [**mergeEntity**](KnowledgeApi.md#mergeentityoperation) | **POST** /api/v1/entities/{id}/merge | 合并重复 Entity |
| [**rebuildEntityGraph**](KnowledgeApi.md#rebuildentitygraph) | **POST** /api/v1/entity-graph/rebuild | 全量重建 Entity 关系图投影 |
| [**registerFederatedWiki**](KnowledgeApi.md#registerfederatedwiki) | **POST** /api/v1/federated-wikis | 登记远端 Wiki |
| [**removeEntityAlias**](KnowledgeApi.md#removeentityalias) | **DELETE** /api/v1/entities/{id}/aliases/{alias_id} | 删除 Entity 别名 |
| [**removeEntityLabel**](KnowledgeApi.md#removeentitylabel) | **DELETE** /api/v1/entities/{id}/labels | 删除 Entity 标签 |
| [**removePageEntityBinding**](KnowledgeApi.md#removepageentitybinding) | **DELETE** /api/v1/pages/{id}/entity-bindings/{entity_id} | 删除 Page 的 Entity 绑定 |
| [**rollbackEntityMerge**](KnowledgeApi.md#rollbackentitymerge) | **POST** /api/v1/entity-merges/{id}/rollback | 补偿回滚 Entity 合并 |
| [**scanFactConsistency**](KnowledgeApi.md#scanfactconsistency) | **POST** /api/v1/fact-consistency-scans | 全量重建事实一致性问题 |
| [**setPageEntityBinding**](KnowledgeApi.md#setpageentitybindingoperation) | **PUT** /api/v1/pages/{id}/entity-bindings | 设置 Page 的 Entity 绑定 |
| [**setPrimaryEntityLabel**](KnowledgeApi.md#setprimaryentitylabeloperation) | **PUT** /api/v1/entities/{id}/labels/primary | 设置 Entity 主标签 |
| [**updateClaimVerification**](KnowledgeApi.md#updateclaimverificationoperation) | **PUT** /api/v1/claims/{id}/verification | 更新 Claim 人工/AI 核验状态 |
| [**updateFederatedWiki**](KnowledgeApi.md#updatefederatedwikioperation) | **PUT** /api/v1/federated-wikis/{id} | 更新远端 Wiki 配置 |
| [**updateFederationLink**](KnowledgeApi.md#updatefederationlink) | **PUT** /api/v1/federation-links/{id} | 更新或弃用 Entity Federation 映射 |



## addEntityAlias

> EntityAlias addEntityAlias(id, addEntityAliasRequest)

新增 Entity 别名

编辑者新增规范化去重的别名、历史名称、缩写或导入名称。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { AddEntityAliasOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // AddEntityAliasRequest
    addEntityAliasRequest: ...,
  } satisfies AddEntityAliasOperationRequest;

  try {
    const data = await api.addEntityAlias(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **addEntityAliasRequest** | [AddEntityAliasRequest](AddEntityAliasRequest.md) |  | |

### Return type

[**EntityAlias**](EntityAlias.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 已创建的别名 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## addEntityLabel

> EntityLabel addEntityLabel(id, addEntityLabelRequest)

新增 Entity 标签

编辑者为活动 Entity 新增多语言标签；权威写入、审计与投影失效在同一事务提交。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { AddEntityLabelOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // AddEntityLabelRequest
    addEntityLabelRequest: ...,
  } satisfies AddEntityLabelOperationRequest;

  try {
    const data = await api.addEntityLabel(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **addEntityLabelRequest** | [AddEntityLabelRequest](AddEntityLabelRequest.md) |  | |

### Return type

[**EntityLabel**](EntityLabel.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 已创建的标签 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createEntityFederationLink

> EntityFederationLink createEntityFederationLink(id, createEntityFederationLinkRequest)

为 Entity 创建跨 Wiki 映射

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { CreateEntityFederationLinkOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // CreateEntityFederationLinkRequest
    createEntityFederationLinkRequest: ...,
  } satisfies CreateEntityFederationLinkOperationRequest;

  try {
    const data = await api.createEntityFederationLink(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **createEntityFederationLinkRequest** | [CreateEntityFederationLinkRequest](CreateEntityFederationLinkRequest.md) |  | |

### Return type

[**EntityFederationLink**](EntityFederationLink.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 已创建的映射 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getCitation

> CitationDetail getCitation(id)

Citation 详情与证据定位链

匿名读取不可变 Citation，并沿 SourceVersion 定位到 Source、可选 Chunk 与外部资源 URL。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { GetCitationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetCitationRequest;

  try {
    const data = await api.getCitation(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |

### Return type

[**CitationDetail**](CitationDetail.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Citation 只读详情及完整定位上下文 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getClaim

> ClaimDetail getClaim(id)

Claim 详情

匿名读取 Claim 的谓词、结构化值、业务/验证状态、取代链及 Citation 绑定。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { GetClaimRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetClaimRequest;

  try {
    const data = await api.getClaim(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |

### Return type

[**ClaimDetail**](ClaimDetail.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Claim 只读详情 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getEntity

> EntityDetail getEntity(id)

Entity 详情

匿名读取 Entity 的稳定身份、类型、状态、标签与别名；本端点不提供写操作。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { GetEntityRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetEntityRequest;

  try {
    const data = await api.getEntity(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |

### Return type

[**EntityDetail**](EntityDetail.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Entity 只读详情 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getEntityGraph

> EntityGraph getEntityGraph(id, direction, propertyKey, depth, maxNodes)

查询 Entity 关系图

匿名读取由当前已发布 entity-valued Claim 派生的关系边投影。 查询不扫描 Claim 或页面 AST；遍历深度最多 3、节点最多 100、边最多 250， 并在达到硬上限时返回 truncated&#x3D;true。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { GetEntityGraphRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // 'outbound' | 'inbound' | 'both' (optional)
    direction: direction_example,
    // string | 只沿指定 Property 的边遍历 (optional)
    propertyKey: propertyKey_example,
    // number (optional)
    depth: 56,
    // number (optional)
    maxNodes: 56,
  } satisfies GetEntityGraphRequest;

  try {
    const data = await api.getEntityGraph(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **direction** | `outbound`, `inbound`, `both` |  | [Optional] [Defaults to `&#39;both&#39;`] [Enum: outbound, inbound, both] |
| **propertyKey** | `string` | 只沿指定 Property 的边遍历 | [Optional] [Defaults to `undefined`] |
| **depth** | `number` |  | [Optional] [Defaults to `1`] |
| **maxNodes** | `number` |  | [Optional] [Defaults to `60`] |

### Return type

[**EntityGraph**](EntityGraph.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 有界 Entity 子图 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getEntityMerge

> EntityMergeResult getEntityMerge(id)

读取 Entity 的最近合并记录

匿名读取源 Entity 的最近一次合并及其不可变标签、Claim 映射账本。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { GetEntityMergeRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetEntityMergeRequest;

  try {
    const data = await api.getEntityMerge(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |

### Return type

[**EntityMergeResult**](EntityMergeResult.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Entity 合并记录 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listEntities

> EntityListPage listEntities(cursor, pageSize, q, typeKey, status)

浏览与检索 Entity 目录

匿名读取当前 Wiki 的稳定 Entity 身份。查询匹配 canonical key、标签与别名； 游标按 updated_at、id 稳定排序。all 状态含 active 与 merged，但始终排除 deleted。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { ListEntitiesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
    // string | 可选名称、别名或 canonical key 查询 (optional)
    q: q_example,
    // string | 精确 EntityType key；不传时不过滤 (optional)
    typeKey: typeKey_example,
    // 'active' | 'merged' | 'all' (optional)
    status: status_example,
  } satisfies ListEntitiesRequest;

  try {
    const data = await api.listEntities(body);
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
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |
| **q** | `string` | 可选名称、别名或 canonical key 查询 | [Optional] [Defaults to `undefined`] |
| **typeKey** | `string` | 精确 EntityType key；不传时不过滤 | [Optional] [Defaults to `undefined`] |
| **status** | `active`, `merged`, `all` |  | [Optional] [Defaults to `&#39;active&#39;`] [Enum: active, merged, all] |

### Return type

[**EntityListPage**](EntityListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Entity 目录页 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listEntityFederationLinks

> EntityFederationLinkListPage listEntityFederationLinks(id, status)

列出一个 Entity 的跨 Wiki 映射

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { ListEntityFederationLinksRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // 'active' | 'deprecated' | 'all' (optional)
    status: status_example,
  } satisfies ListEntityFederationLinksRequest;

  try {
    const data = await api.listEntityFederationLinks(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **status** | `active`, `deprecated`, `all` |  | [Optional] [Defaults to `&#39;active&#39;`] [Enum: active, deprecated, all] |

### Return type

[**EntityFederationLinkListPage**](EntityFederationLinkListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Entity 的 Federation 映射 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listFactConsistencyIssues

> FactConsistencyIssueListPage listFactConsistencyIssues(status, issueType, cursor, pageSize)

列出自动事实一致性问题

从当前已发布 Claim、Property 约束与 ClaimSource 派生的可重建问题队列。 默认只返回 open 问题；需要 reviewer 或 admin 权限。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { ListFactConsistencyIssuesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // 'open' | 'resolved' | 'all' (optional)
    status: status_example,
    // 'single_value_conflict' | 'multiple_preferred_values' | 'verified_without_support' | 'merged_entity_target' | 'evidence_disagreement' (optional)
    issueType: issueType_example,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListFactConsistencyIssuesRequest;

  try {
    const data = await api.listFactConsistencyIssues(body);
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
| **status** | `open`, `resolved`, `all` |  | [Optional] [Defaults to `&#39;open&#39;`] [Enum: open, resolved, all] |
| **issueType** | `single_value_conflict`, `multiple_preferred_values`, `verified_without_support`, `merged_entity_target`, `evidence_disagreement` |  | [Optional] [Defaults to `undefined`] [Enum: single_value_conflict, multiple_preferred_values, verified_without_support, merged_entity_target, evidence_disagreement] |
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**FactConsistencyIssueListPage**](FactConsistencyIssueListPage.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 一页事实一致性问题 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listFederatedWikis

> FederatedWikiList listFederatedWikis(includeDisabled)

列出跨 Wiki 联邦目录

匿名读取当前 Wiki 已登记的远端 Wiki、信任等级与 Entity 链接模板。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { ListFederatedWikisRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // boolean | 是否包含已停用的远端 Wiki (optional)
    includeDisabled: true,
  } satisfies ListFederatedWikisRequest;

  try {
    const data = await api.listFederatedWikis(body);
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
| **includeDisabled** | `boolean` | 是否包含已停用的远端 Wiki | [Optional] [Defaults to `false`] |

### Return type

[**FederatedWikiList**](FederatedWikiList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 远端 Wiki 目录 |  * X-Request-ID -  <br>  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listFederationLinks

> EntityFederationLinkListPage listFederationLinks(cursor, pageSize, q, remoteWikiId, status)

浏览跨 Wiki Entity 映射

匿名分页读取本地 Entity 与远端 Entity 的可追溯映射。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { ListFederationLinksRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
    // string | 匹配本地或远端 key、标签与远端 Entity ID (optional)
    q: q_example,
    // string (optional)
    remoteWikiId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // 'active' | 'deprecated' | 'all' (optional)
    status: status_example,
  } satisfies ListFederationLinksRequest;

  try {
    const data = await api.listFederationLinks(body);
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
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |
| **q** | `string` | 匹配本地或远端 key、标签与远端 Entity ID | [Optional] [Defaults to `undefined`] |
| **remoteWikiId** | `string` |  | [Optional] [Defaults to `undefined`] |
| **status** | `active`, `deprecated`, `all` |  | [Optional] [Defaults to `&#39;active&#39;`] [Enum: active, deprecated, all] |

### Return type

[**EntityFederationLinkListPage**](EntityFederationLinkListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 一页 Federation 映射 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listPageEntityBindings

> PageEntityBindingList listPageEntityBindings(id)

列出 Page 的 Entity 绑定

匿名读取主实体与显式提及绑定；primary 与 page.primary_entity_id 由数据库延迟约束保证一致。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { ListPageEntityBindingsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new KnowledgeApi();

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies ListPageEntityBindingsRequest;

  try {
    const data = await api.listPageEntityBindings(body);
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
| **id** | `string` | 页面 ID（UUIDv7） | [Defaults to `undefined`] |

### Return type

[**PageEntityBindingList**](PageEntityBindingList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 页面 Entity 绑定 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## mergeEntity

> EntityMergeResult mergeEntity(id, mergeEntityRequest)

合并重复 Entity

仅站点管理员或 system Actor 可触发。合并、审计与 entity.merged Outbox 在同一事务提交；引用修复 Proposal 由 Worker 异步幂等生成。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { MergeEntityOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // MergeEntityRequest
    mergeEntityRequest: ...,
  } satisfies MergeEntityOperationRequest;

  try {
    const data = await api.mergeEntity(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **mergeEntityRequest** | [MergeEntityRequest](MergeEntityRequest.md) |  | |

### Return type

[**EntityMergeResult**](EntityMergeResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 合并完成；同一 source/target 的重复请求返回既有结果 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## rebuildEntityGraph

> EntityGraphRebuildResult rebuildEntityGraph()

全量重建 Entity 关系图投影

管理员运维入口；从已发布 Claim 原子替换当前 Wiki 的全部关系边。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { RebuildEntityGraphRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  try {
    const data = await api.rebuildEntityGraph();
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

[**EntityGraphRebuildResult**](EntityGraphRebuildResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 重建统计 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## registerFederatedWiki

> FederatedWiki registerFederatedWiki(createFederatedWikiRequest)

登记远端 Wiki

管理员登记一个可用于 Entity Federation 的远端 Wiki；wiki_key 在创建后不可变。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { RegisterFederatedWikiRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // CreateFederatedWikiRequest
    createFederatedWikiRequest: ...,
  } satisfies RegisterFederatedWikiRequest;

  try {
    const data = await api.registerFederatedWiki(body);
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
| **createFederatedWikiRequest** | [CreateFederatedWikiRequest](CreateFederatedWikiRequest.md) |  | |

### Return type

[**FederatedWiki**](FederatedWiki.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 已登记的远端 Wiki |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## removeEntityAlias

> removeEntityAlias(id, aliasId)

删除 Entity 别名

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { RemoveEntityAliasRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string
    aliasId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies RemoveEntityAliasRequest;

  try {
    const data = await api.removeEntityAlias(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **aliasId** | `string` |  | [Defaults to `undefined`] |

### Return type

`void` (Empty response body)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | 别名已删除 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## removeEntityLabel

> removeEntityLabel(id, language, label)

删除 Entity 标签

编辑者删除精确匹配的标签；实体始终至少保留一个主标签。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { RemoveEntityLabelRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string
    language: language_example,
    // string
    label: label_example,
  } satisfies RemoveEntityLabelRequest;

  try {
    const data = await api.removeEntityLabel(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **language** | `string` |  | [Defaults to `undefined`] |
| **label** | `string` |  | [Defaults to `undefined`] |

### Return type

`void` (Empty response body)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | 标签已删除 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## removePageEntityBinding

> removePageEntityBinding(id, entityId, role)

删除 Page 的 Entity 绑定

删除 primary 时同事务清空 page.primary_entity_id。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { RemovePageEntityBindingRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string
    entityId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // 'primary' | 'mentioned'
    role: role_example,
  } satisfies RemovePageEntityBindingRequest;

  try {
    const data = await api.removePageEntityBinding(body);
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
| **id** | `string` | 页面 ID（UUIDv7） | [Defaults to `undefined`] |
| **entityId** | `string` |  | [Defaults to `undefined`] |
| **role** | `primary`, `mentioned` |  | [Defaults to `undefined`] [Enum: primary, mentioned] |

### Return type

`void` (Empty response body)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | 绑定已删除 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## rollbackEntityMerge

> RollbackEntityMergeResult rollbackEntityMerge(id)

补偿回滚 Entity 合并

仅管理员可执行。服务端在一个事务中恢复源 Entity，补偿合并产生的 Claim， 移除只由该合并复制且未被后续修改的标签，并追加审计；任何状态漂移都会拒绝整笔回滚。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { RollbackEntityMergeRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies RollbackEntityMergeRequest;

  try {
    const data = await api.rollbackEntityMerge(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |

### Return type

[**RollbackEntityMergeResult**](RollbackEntityMergeResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Entity 合并补偿结果；重复请求幂等返回 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## scanFactConsistency

> FactConsistencyScanResult scanFactConsistency()

全量重建事实一致性问题

管理员按 Entity 扫描当前 Wiki；发现的问题幂等更新， 已消失的问题标记 resolved，不修改任何权威 Claim。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { ScanFactConsistencyRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  try {
    const data = await api.scanFactConsistency();
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

[**FactConsistencyScanResult**](FactConsistencyScanResult.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 本次扫描统计 |  * X-Request-ID -  <br>  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## setPageEntityBinding

> PageEntityBinding setPageEntityBinding(id, setPageEntityBindingRequest)

设置 Page 的 Entity 绑定

编辑者设置绑定。primary 角色会原子替换现有主绑定并同步 page.primary_entity_id； 完全相同的请求幂等成功。mentioned 角色按 (page, entity, role) 去重。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { SetPageEntityBindingOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | 页面 ID（UUIDv7）
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // SetPageEntityBindingRequest
    setPageEntityBindingRequest: ...,
  } satisfies SetPageEntityBindingOperationRequest;

  try {
    const data = await api.setPageEntityBinding(body);
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
| **id** | `string` | 页面 ID（UUIDv7） | [Defaults to `undefined`] |
| **setPageEntityBindingRequest** | [SetPageEntityBindingRequest](SetPageEntityBindingRequest.md) |  | |

### Return type

[**PageEntityBinding**](PageEntityBinding.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前绑定 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## setPrimaryEntityLabel

> setPrimaryEntityLabel(id, setPrimaryEntityLabelRequest)

设置 Entity 主标签

编辑者在指定语言内切换主标签；标签必须已存在。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { SetPrimaryEntityLabelOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // SetPrimaryEntityLabelRequest
    setPrimaryEntityLabelRequest: ...,
  } satisfies SetPrimaryEntityLabelOperationRequest;

  try {
    const data = await api.setPrimaryEntityLabel(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **setPrimaryEntityLabelRequest** | [SetPrimaryEntityLabelRequest](SetPrimaryEntityLabelRequest.md) |  | |

### Return type

`void` (Empty response body)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | 主标签已更新；已是主标签时幂等成功 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## updateClaimVerification

> updateClaimVerification(id, updateClaimVerificationRequest)

更新 Claim 人工/AI 核验状态

reviewer 或 admin 可提交核验状态。领域服务仍按 Actor 类型执行防御性矩阵： human 可设置全部状态，AI 仅可设置 ai_checked；变更与审计、精准重渲染事件原子提交。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { UpdateClaimVerificationOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // UpdateClaimVerificationRequest
    updateClaimVerificationRequest: ...,
  } satisfies UpdateClaimVerificationOperationRequest;

  try {
    const data = await api.updateClaimVerification(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **updateClaimVerificationRequest** | [UpdateClaimVerificationRequest](UpdateClaimVerificationRequest.md) |  | |

### Return type

`void` (Empty response body)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | 核验状态已更新；状态未变化时幂等成功 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## updateFederatedWiki

> FederatedWiki updateFederatedWiki(id, updateFederatedWikiRequest)

更新远端 Wiki 配置

管理员替换远端 Wiki 的展示、链接模板、信任等级和启停状态；wiki_key 保持不变。

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { UpdateFederatedWikiOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // UpdateFederatedWikiRequest
    updateFederatedWikiRequest: ...,
  } satisfies UpdateFederatedWikiOperationRequest;

  try {
    const data = await api.updateFederatedWiki(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **updateFederatedWikiRequest** | [UpdateFederatedWikiRequest](UpdateFederatedWikiRequest.md) |  | |

### Return type

[**FederatedWiki**](FederatedWiki.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 更新后的远端 Wiki |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## updateFederationLink

> EntityFederationLink updateFederationLink(id, updateEntityFederationLinkRequest)

更新或弃用 Entity Federation 映射

### Example

```ts
import {
  Configuration,
  KnowledgeApi,
} from '';
import type { UpdateFederationLinkRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new KnowledgeApi(config);

  const body = {
    // string | Entity、Claim 或 Citation 稳定 ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // UpdateEntityFederationLinkRequest
    updateEntityFederationLinkRequest: ...,
  } satisfies UpdateFederationLinkRequest;

  try {
    const data = await api.updateFederationLink(body);
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
| **id** | `string` | Entity、Claim 或 Citation 稳定 ID | [Defaults to `undefined`] |
| **updateEntityFederationLinkRequest** | [UpdateEntityFederationLinkRequest](UpdateEntityFederationLinkRequest.md) |  | |

### Return type

[**EntityFederationLink**](EntityFederationLink.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 更新后的映射 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

