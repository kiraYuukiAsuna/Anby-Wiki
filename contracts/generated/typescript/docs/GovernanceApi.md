# GovernanceApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**addProposalOperation**](GovernanceApi.md#addproposaloperation) | **POST** /api/v1/proposals/{id}/operations | 向 draft Proposal 追加 v1 Operation |
| [**applyNextBulkReviewWave**](GovernanceApi.md#applynextbulkreviewwave) | **POST** /api/v1/bulk-review-batches/{id}/apply-next-wave | 逐 Proposal 调用既有 Apply 边界应用下一固定 wave |
| [**applyProposal**](GovernanceApi.md#applyproposal) | **POST** /api/v1/proposals/{id}/apply | 原子应用已批准 Proposal |
| [**archiveRevisionSnapshots**](GovernanceApi.md#archiverevisionsnapshotsoperation) | **POST** /api/v1/revision-storage/archive | 手动执行一批到期 Revision 快照归档 |
| [**assignChangeTag**](GovernanceApi.md#assignchangetagoperation) | **POST** /api/v1/change-tags/{id}/assignments | 给 Revision、Proposal 或 AuditEvent 追加不可变标签 |
| [**createBulkReviewBatch**](GovernanceApi.md#createbulkreviewbatchoperation) | **POST** /api/v1/bulk-review-batches | 创建并冻结批量风险审核的抽样集合与 Apply wave |
| [**createChangeTag**](GovernanceApi.md#createchangetagoperation) | **POST** /api/v1/change-tags | 创建不可变 ChangeTag |
| [**createPageProtection**](GovernanceApi.md#createpageprotectionoperation) | **POST** /api/v1/page-protections | 创建页面或标题范围的保护规则 |
| [**createProposal**](GovernanceApi.md#createproposaloperation) | **POST** /api/v1/proposals | 幂等创建 Proposal 草稿 |
| [**decideBulkReviewProposal**](GovernanceApi.md#decidebulkreviewproposal) | **POST** /api/v1/bulk-review-batches/{id}/proposals/{proposal_id}/decision | 在批次中批准或拒绝单个 Proposal |
| [**decideReviewTask**](GovernanceApi.md#decidereviewtask) | **POST** /api/v1/review-tasks/{id}/decision | 人工批准或拒绝 ReviewTask |
| [**deletePageProtection**](GovernanceApi.md#deletepageprotection) | **DELETE** /api/v1/page-protections/{id} | 撤销 PageProtection（审计历史保留） |
| [**finalizeBulkReviewBatch**](GovernanceApi.md#finalizebulkreviewbatch) | **POST** /api/v1/bulk-review-batches/{id}/finalize | 抽样通过后批准未抽样 Proposal 并冻结审核结果 |
| [**getBulkReviewBatch**](GovernanceApi.md#getbulkreviewbatch) | **GET** /api/v1/bulk-review-batches/{id} | 读取批量审核、Proposal 决策与固定 wave |
| [**getProposal**](GovernanceApi.md#getproposal) | **GET** /api/v1/proposals/{id} | 读取 Proposal、Operation 与冲突 |
| [**getRevisionStorageStats**](GovernanceApi.md#getrevisionstoragestats) | **GET** /api/v1/revision-storage | 查看 Revision 快照冷热分层状态 |
| [**listAITrustProfiles**](GovernanceApi.md#listaitrustprofiles) | **GET** /api/v1/ai-trust-profiles | 列出 AI/Import Actor 的信任与抽样策略 |
| [**listAuditEvents**](GovernanceApi.md#listauditevents) | **GET** /api/v1/audit-events | 分页查询当前 Wiki 的不可变审计流 |
| [**listBulkReviewAuditEvents**](GovernanceApi.md#listbulkreviewauditevents) | **GET** /api/v1/bulk-review-batches/{id}/audit-events | 查询批量审核、决策、暂停和 wave Apply 审计 |
| [**listBulkReviewBatches**](GovernanceApi.md#listbulkreviewbatches) | **GET** /api/v1/bulk-review-batches | 查询当前 Wiki 的批量审核批次 |
| [**listChangeTags**](GovernanceApi.md#listchangetags) | **GET** /api/v1/change-tags | 列出不可变变更标签词表 |
| [**listPageProtections**](GovernanceApi.md#listpageprotections) | **GET** /api/v1/page-protections | 列出当前 Wiki 的 PageProtection |
| [**listProposals**](GovernanceApi.md#listproposals) | **GET** /api/v1/proposals | 列出当前 Actor 创建的 Proposal |
| [**listReviewTasks**](GovernanceApi.md#listreviewtasks) | **GET** /api/v1/review-tasks | 人工审核队列 |
| [**listRoles**](GovernanceApi.md#listroles) | **GET** /api/v1/roles | 列出 PageProtection 可用角色 |
| [**mergeProposalToWorkingDocument**](GovernanceApi.md#mergeproposaltoworkingdocumentoperation) | **POST** /api/v1/proposals/{id}/merge-to-working-document | 以 sequence CAS 将已验证的 Proposal Yjs delta 合并到工作副本 |
| [**pauseBulkReviewBatch**](GovernanceApi.md#pausebulkreviewbatch) | **POST** /api/v1/bulk-review-batches/{id}/pause | 暂停后续 Proposal Apply |
| [**previewProposal**](GovernanceApi.md#previewproposal) | **GET** /api/v1/proposals/{id}/preview | 无权威写入地预览 Base、Current 与 Proposed |
| [**resolveMergeConflict**](GovernanceApi.md#resolvemergeconflictoperation) | **POST** /api/v1/proposals/{id}/conflicts/{conflict_id}/resolution | 记录单个 MergeConflict 的人工决议 |
| [**resumeBulkReviewBatch**](GovernanceApi.md#resumebulkreviewbatch) | **POST** /api/v1/bulk-review-batches/{id}/resume | 恢复后续 Proposal Apply |
| [**rollbackChangeBatch**](GovernanceApi.md#rollbackchangebatch) | **POST** /api/v1/change-batches/{id}/rollback | 以新版本补偿回滚 ChangeBatch |
| [**submitProposal**](GovernanceApi.md#submitproposal) | **POST** /api/v1/proposals/{id}/submit | 提交并执行风险策略 |
| [**updateAITrustProfile**](GovernanceApi.md#updateaitrustprofileoperation) | **PUT** /api/v1/ai-trust-profiles/{actor_id} | 更新 AI 信任等级与低风险人工抽样比例 |



## addProposalOperation

> ProposalOperationRecord addProposalOperation(id, proposalOperationV1)

向 draft Proposal 追加 v1 Operation

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { AddProposalOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // ProposalOperationV1
    proposalOperationV1: ...,
  } satisfies AddProposalOperationRequest;

  try {
    const data = await api.addProposalOperation(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |
| **proposalOperationV1** | [ProposalOperationV1](ProposalOperationV1.md) |  | |

### Return type

[**ProposalOperationRecord**](ProposalOperationRecord.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 已按服务端序号追加 |  -  |
| **401** | 未认证 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## applyNextBulkReviewWave

> BulkReviewWaveResult applyNextBulkReviewWave(id)

逐 Proposal 调用既有 Apply 边界应用下一固定 wave

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ApplyNextBulkReviewWaveRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies ApplyNextBulkReviewWaveRequest;

  try {
    const data = await api.applyNextBulkReviewWave(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**BulkReviewWaveResult**](BulkReviewWaveResult.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | wave 结果；每个成功项拥有自己的 ChangeBatch |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## applyProposal

> ApplyProposalResult applyProposal(id)

原子应用已批准 Proposal

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ApplyProposalRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies ApplyProposalRequest;

  try {
    const data = await api.applyProposal(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**ApplyProposalResult**](ApplyProposalResult.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | ChangeBatch 与创建的权威版本 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## archiveRevisionSnapshots

> SnapshotArchiveResult archiveRevisionSnapshots(archiveRevisionSnapshotsRequest)

手动执行一批到期 Revision 快照归档

仅管理员可执行。对象上传与校验完成后，Page 领域服务才把非当前 hot 快照原子切换到 cold；当前 Revision 永远保留在 hot tier。

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ArchiveRevisionSnapshotsOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // ArchiveRevisionSnapshotsRequest
    archiveRevisionSnapshotsRequest: ...,
  } satisfies ArchiveRevisionSnapshotsOperationRequest;

  try {
    const data = await api.archiveRevisionSnapshots(body);
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
| **archiveRevisionSnapshotsRequest** | [ArchiveRevisionSnapshotsRequest](ArchiveRevisionSnapshotsRequest.md) |  | |

### Return type

[**SnapshotArchiveResult**](SnapshotArchiveResult.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 本批归档结果 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **503** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## assignChangeTag

> ChangeTagAssignmentResult assignChangeTag(id, assignChangeTagRequest)

给 Revision、Proposal 或 AuditEvent 追加不可变标签

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { AssignChangeTagOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | ChangeTag ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // AssignChangeTagRequest
    assignChangeTagRequest: ...,
  } satisfies AssignChangeTagOperationRequest;

  try {
    const data = await api.assignChangeTag(body);
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
| **id** | `string` | ChangeTag ID | [Defaults to `undefined`] |
| **assignChangeTagRequest** | [AssignChangeTagRequest](AssignChangeTagRequest.md) |  | |

### Return type

[**ChangeTagAssignmentResult**](ChangeTagAssignmentResult.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 已赋予标签；重复请求幂等成功 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createBulkReviewBatch

> BulkReviewBatch createBulkReviewBatch(createBulkReviewBatchRequest)

创建并冻结批量风险审核的抽样集合与 Apply wave

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { CreateBulkReviewBatchOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // CreateBulkReviewBatchRequest
    createBulkReviewBatchRequest: ...,
  } satisfies CreateBulkReviewBatchOperationRequest;

  try {
    const data = await api.createBulkReviewBatch(body);
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
| **createBulkReviewBatchRequest** | [CreateBulkReviewBatchRequest](CreateBulkReviewBatchRequest.md) |  | |

### Return type

[**BulkReviewBatch**](BulkReviewBatch.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 独立审核聚合；不创建 ChangeBatch |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createChangeTag

> ChangeTag createChangeTag(createChangeTagRequest)

创建不可变 ChangeTag

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { CreateChangeTagOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // CreateChangeTagRequest
    createChangeTagRequest: ...,
  } satisfies CreateChangeTagOperationRequest;

  try {
    const data = await api.createChangeTag(body);
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
| **createChangeTagRequest** | [CreateChangeTagRequest](CreateChangeTagRequest.md) |  | |

### Return type

[**ChangeTag**](ChangeTag.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 已创建标签 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createPageProtection

> PageProtection createPageProtection(createPageProtectionRequest)

创建页面或标题范围的保护规则

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { CreatePageProtectionOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // CreatePageProtectionRequest
    createPageProtectionRequest: ...,
  } satisfies CreatePageProtectionOperationRequest;

  try {
    const data = await api.createPageProtection(body);
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
| **createPageProtectionRequest** | [CreatePageProtectionRequest](CreatePageProtectionRequest.md) |  | |

### Return type

[**PageProtection**](PageProtection.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | 已创建保护规则 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## createProposal

> Proposal createProposal(idempotencyKey, createProposalRequest)

幂等创建 Proposal 草稿

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { CreateProposalOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | 变更类请求的幂等键（客户端生成的 UUID）。 服务端对相同 Actor + 幂等键的重复请求返回首次处理结果，不重复执行。
    idempotencyKey: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // CreateProposalRequest
    createProposalRequest: ...,
  } satisfies CreateProposalOperationRequest;

  try {
    const data = await api.createProposal(body);
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
| **idempotencyKey** | `string` | 变更类请求的幂等键（客户端生成的 UUID）。 服务端对相同 Actor + 幂等键的重复请求返回首次处理结果，不重复执行。  | [Defaults to `undefined`] |
| **createProposalRequest** | [CreateProposalRequest](CreateProposalRequest.md) |  | |

### Return type

[**Proposal**](Proposal.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | Proposal 草稿 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## decideBulkReviewProposal

> BulkReviewBatch decideBulkReviewProposal(id, proposalId, bulkReviewDecisionRequest)

在批次中批准或拒绝单个 Proposal

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { DecideBulkReviewProposalRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string
    proposalId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // BulkReviewDecisionRequest
    bulkReviewDecisionRequest: ...,
  } satisfies DecideBulkReviewProposalRequest;

  try {
    const data = await api.decideBulkReviewProposal(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |
| **proposalId** | `string` |  | [Defaults to `undefined`] |
| **bulkReviewDecisionRequest** | [BulkReviewDecisionRequest](BulkReviewDecisionRequest.md) |  | |

### Return type

[**BulkReviewBatch**](BulkReviewBatch.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 更新后的批量审核 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## decideReviewTask

> Proposal decideReviewTask(id, reviewDecisionRequest)

人工批准或拒绝 ReviewTask

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { DecideReviewTaskRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // ReviewDecisionRequest
    reviewDecisionRequest: ...,
  } satisfies DecideReviewTaskRequest;

  try {
    const data = await api.decideReviewTask(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |
| **reviewDecisionRequest** | [ReviewDecisionRequest](ReviewDecisionRequest.md) |  | |

### Return type

[**Proposal**](Proposal.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 更新后的 Proposal |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## deletePageProtection

> deletePageProtection(id)

撤销 PageProtection（审计历史保留）

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { DeletePageProtectionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies DeletePageProtectionRequest;

  try {
    const data = await api.deletePageProtection(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

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
| **204** | 已撤销 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## finalizeBulkReviewBatch

> BulkReviewBatch finalizeBulkReviewBatch(id)

抽样通过后批准未抽样 Proposal 并冻结审核结果

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { FinalizeBulkReviewBatchRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies FinalizeBulkReviewBatchRequest;

  try {
    const data = await api.finalizeBulkReviewBatch(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**BulkReviewBatch**](BulkReviewBatch.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 已进入 ready 或全部拒绝后 completed |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getBulkReviewBatch

> BulkReviewBatch getBulkReviewBatch(id)

读取批量审核、Proposal 决策与固定 wave

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { GetBulkReviewBatchRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetBulkReviewBatchRequest;

  try {
    const data = await api.getBulkReviewBatch(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**BulkReviewBatch**](BulkReviewBatch.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 批量审核详情 |  -  |
| **401** | 未认证 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getProposal

> Proposal getProposal(id)

读取 Proposal、Operation 与冲突

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { GetProposalRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetProposalRequest;

  try {
    const data = await api.getProposal(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**Proposal**](Proposal.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Proposal 详情 |  -  |
| **401** | 未认证 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getRevisionStorageStats

> RevisionStorageStats getRevisionStorageStats()

查看 Revision 快照冷热分层状态

仅管理员可见。统计不可变 ContentSnapshot 的物理存储层、 到期候选数与归档能力，不返回 AST 正文或对象存储键。

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { GetRevisionStorageStatsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  try {
    const data = await api.getRevisionStorageStats();
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

[**RevisionStorageStats**](RevisionStorageStats.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前冷热分层统计 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listAITrustProfiles

> AITrustProfileList listAITrustProfiles()

列出 AI/Import Actor 的信任与抽样策略

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ListAITrustProfilesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  try {
    const data = await api.listAITrustProfiles();
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

[**AITrustProfileList**](AITrustProfileList.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 未配置 Actor 以 untrusted/100% 的保守默认值返回 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listAuditEvents

> AuditEventListPage listAuditEvents(cursor, pageSize, eventType, aggregateType, changeBatchId, tagKey)

分页查询当前 Wiki 的不可变审计流

需要 reviewer 或 admin 角色；响应 payload 只包含领域服务写入的结构化审计数据。

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ListAuditEventsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
    // string (optional)
    eventType: eventType_example,
    // string (optional)
    aggregateType: aggregateType_example,
    // string (optional)
    changeBatchId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string (optional)
    tagKey: tagKey_example,
  } satisfies ListAuditEventsRequest;

  try {
    const data = await api.listAuditEvents(body);
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
| **eventType** | `string` |  | [Optional] [Defaults to `undefined`] |
| **aggregateType** | `string` |  | [Optional] [Defaults to `undefined`] |
| **changeBatchId** | `string` |  | [Optional] [Defaults to `undefined`] |
| **tagKey** | `string` |  | [Optional] [Defaults to `undefined`] |

### Return type

[**AuditEventListPage**](AuditEventListPage.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前 Wiki 审计事件 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listBulkReviewAuditEvents

> BulkReviewAuditEventList listBulkReviewAuditEvents(id)

查询批量审核、决策、暂停和 wave Apply 审计

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ListBulkReviewAuditEventsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies ListBulkReviewAuditEventsRequest;

  try {
    const data = await api.listBulkReviewAuditEvents(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**BulkReviewAuditEventList**](BulkReviewAuditEventList.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 按创建时间排序的审计事件 |  -  |
| **401** | 未认证 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listBulkReviewBatches

> BulkReviewBatchListPage listBulkReviewBatches(status, cursor, pageSize)

查询当前 Wiki 的批量审核批次

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ListBulkReviewBatchesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // 'reviewing' | 'ready' | 'applying' | 'paused' | 'completed' (optional)
    status: status_example,
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListBulkReviewBatchesRequest;

  try {
    const data = await api.listBulkReviewBatches(body);
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
| **status** | `reviewing`, `ready`, `applying`, `paused`, `completed` |  | [Optional] [Defaults to `undefined`] [Enum: reviewing, ready, applying, paused, completed] |
| **cursor** | `string` | 上一页响应返回的 next_cursor；首页不传 | [Optional] [Defaults to `undefined`] |
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**BulkReviewBatchListPage**](BulkReviewBatchListPage.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 按创建时间倒序的批量审核批次 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listChangeTags

> ChangeTagList listChangeTags()

列出不可变变更标签词表

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ListChangeTagsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new GovernanceApi();

  try {
    const data = await api.listChangeTags();
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

[**ChangeTagList**](ChangeTagList.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 全局 ChangeTag 词表 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listPageProtections

> PageProtectionList listPageProtections(pageId, includeExpired)

列出当前 Wiki 的 PageProtection

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ListPageProtectionsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string (optional)
    pageId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // boolean (optional)
    includeExpired: true,
  } satisfies ListPageProtectionsRequest;

  try {
    const data = await api.listPageProtections(body);
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
| **pageId** | `string` |  | [Optional] [Defaults to `undefined`] |
| **includeExpired** | `boolean` |  | [Optional] [Defaults to `false`] |

### Return type

[**PageProtectionList**](PageProtectionList.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | PageProtection 列表 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listProposals

> ProposalListPage listProposals(cursor, pageSize, status, targetType)

列出当前 Actor 创建的 Proposal

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ListProposalsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
    // 'draft' | 'submitted' | 'in_review' | 'approved' | 'rejected' | 'conflicted' | 'applying' | 'applied' | 'failed' | 'rolled_back' (optional)
    status: status_example,
    // 'page' | 'entity' | 'claim' | 'collection' | 'external_resource' (optional)
    targetType: targetType_example,
  } satisfies ListProposalsRequest;

  try {
    const data = await api.listProposals(body);
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
| **status** | `draft`, `submitted`, `in_review`, `approved`, `rejected`, `conflicted`, `applying`, `applied`, `failed`, `rolled_back` |  | [Optional] [Defaults to `undefined`] [Enum: draft, submitted, in_review, approved, rejected, conflicted, applying, applied, failed, rolled_back] |
| **targetType** | `page`, `entity`, `claim`, `collection`, `external_resource` |  | [Optional] [Defaults to `undefined`] [Enum: page, entity, claim, collection, external_resource] |

### Return type

[**ProposalListPage**](ProposalListPage.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前 Actor 的 Proposal 游标分页结果 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listReviewTasks

> ReviewTaskList listReviewTasks(pageSize)

人工审核队列

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ListReviewTasksRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
  } satisfies ListReviewTasksRequest;

  try {
    const data = await api.listReviewTasks(body);
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
| **pageSize** | `number` | 每页条数，默认 20，最大 100 | [Optional] [Defaults to `20`] |

### Return type

[**ReviewTaskList**](ReviewTaskList.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 待审核任务 |  -  |
| **401** | 未认证 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listRoles

> RoleList listRoles()

列出 PageProtection 可用角色

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ListRolesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  try {
    const data = await api.listRoles();
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

[**RoleList**](RoleList.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 角色目录 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## mergeProposalToWorkingDocument

> MergeProposalToWorkingDocumentResult mergeProposalToWorkingDocument(id, mergeProposalToWorkingDocumentRequest)

以 sequence CAS 将已验证的 Proposal Yjs delta 合并到工作副本

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { MergeProposalToWorkingDocumentOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // MergeProposalToWorkingDocumentRequest
    mergeProposalToWorkingDocumentRequest: ...,
  } satisfies MergeProposalToWorkingDocumentOperationRequest;

  try {
    const data = await api.mergeProposalToWorkingDocument(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |
| **mergeProposalToWorkingDocumentRequest** | [MergeProposalToWorkingDocumentRequest](MergeProposalToWorkingDocumentRequest.md) |  | |

### Return type

[**MergeProposalToWorkingDocumentResult**](MergeProposalToWorkingDocumentResult.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Proposal 已合并到 WorkingDocument |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## pauseBulkReviewBatch

> BulkReviewBatch pauseBulkReviewBatch(id)

暂停后续 Proposal Apply

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { PauseBulkReviewBatchRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies PauseBulkReviewBatchRequest;

  try {
    const data = await api.pauseBulkReviewBatch(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**BulkReviewBatch**](BulkReviewBatch.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 已暂停，固定 wave 不变 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## previewProposal

> ProposalPreview previewProposal(id)

无权威写入地预览 Base、Current 与 Proposed

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { PreviewProposalRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies PreviewProposalRequest;

  try {
    const data = await api.previewProposal(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**ProposalPreview**](ProposalPreview.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 页面 Proposal 三视图、来源与影响范围 |  -  |
| **401** | 未认证 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## resolveMergeConflict

> Proposal resolveMergeConflict(id, conflictId, resolveMergeConflictRequest)

记录单个 MergeConflict 的人工决议

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ResolveMergeConflictOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // string
    conflictId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // ResolveMergeConflictRequest
    resolveMergeConflictRequest: ...,
  } satisfies ResolveMergeConflictOperationRequest;

  try {
    const data = await api.resolveMergeConflict(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |
| **conflictId** | `string` |  | [Defaults to `undefined`] |
| **resolveMergeConflictRequest** | [ResolveMergeConflictRequest](ResolveMergeConflictRequest.md) |  | |

### Return type

[**Proposal**](Proposal.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 更新后的 Proposal；全部冲突解决后恢复 approved |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## resumeBulkReviewBatch

> BulkReviewBatch resumeBulkReviewBatch(id)

恢复后续 Proposal Apply

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { ResumeBulkReviewBatchRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies ResumeBulkReviewBatchRequest;

  try {
    const data = await api.resumeBulkReviewBatch(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**BulkReviewBatch**](BulkReviewBatch.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 已恢复为 ready |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **404** | 资源不存在 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## rollbackChangeBatch

> RollbackChangeBatchResult rollbackChangeBatch(id)

以新版本补偿回滚 ChangeBatch

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { RollbackChangeBatchRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies RollbackChangeBatchRequest;

  try {
    const data = await api.rollbackChangeBatch(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**RollbackChangeBatchResult**](RollbackChangeBatchResult.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 补偿 Revision/Claim |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## submitProposal

> SubmitProposalResult submitProposal(id)

提交并执行风险策略

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { SubmitProposalRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string | Proposal、ReviewTask 或 ChangeBatch ID
    id: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies SubmitProposalRequest;

  try {
    const data = await api.submitProposal(body);
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
| **id** | `string` | Proposal、ReviewTask 或 ChangeBatch ID | [Defaults to `undefined`] |

### Return type

[**SubmitProposalResult**](SubmitProposalResult.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 自动批准或进入人工队列 |  -  |
| **401** | 未认证 |  -  |
| **409** | 并发冲突（含陈旧基线、幂等键冲突） |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## updateAITrustProfile

> AITrustProfile updateAITrustProfile(actorId, updateAITrustProfileRequest)

更新 AI 信任等级与低风险人工抽样比例

### Example

```ts
import {
  Configuration,
  GovernanceApi,
} from '';
import type { UpdateAITrustProfileOperationRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new GovernanceApi(config);

  const body = {
    // string
    actorId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
    // UpdateAITrustProfileRequest
    updateAITrustProfileRequest: ...,
  } satisfies UpdateAITrustProfileOperationRequest;

  try {
    const data = await api.updateAITrustProfile(body);
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
| **updateAITrustProfileRequest** | [UpdateAITrustProfileRequest](UpdateAITrustProfileRequest.md) |  | |

### Return type

[**AITrustProfile**](AITrustProfile.md)

### Authorization

[sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前信任档案 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **422** | 请求语义可理解但无法处理（如重定向环/重定向链过深） |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

