# GovernanceApi

All URIs are relative to *http://localhost:8000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**addProposalOperation**](GovernanceApi.md#addproposaloperation) | **POST** /api/v1/proposals/{id}/operations | 向 draft Proposal 追加 v1 Operation |
| [**applyNextBulkReviewWave**](GovernanceApi.md#applynextbulkreviewwave) | **POST** /api/v1/bulk-review-batches/{id}/apply-next-wave | 逐 Proposal 调用既有 Apply 边界应用下一固定 wave |
| [**applyProposal**](GovernanceApi.md#applyproposal) | **POST** /api/v1/proposals/{id}/apply | 原子应用已批准 Proposal |
| [**createBulkReviewBatch**](GovernanceApi.md#createbulkreviewbatchoperation) | **POST** /api/v1/bulk-review-batches | 创建并冻结批量风险审核的抽样集合与 Apply wave |
| [**createProposal**](GovernanceApi.md#createproposaloperation) | **POST** /api/v1/proposals | 幂等创建 Proposal 草稿 |
| [**decideBulkReviewProposal**](GovernanceApi.md#decidebulkreviewproposal) | **POST** /api/v1/bulk-review-batches/{id}/proposals/{proposal_id}/decision | 在批次中批准或拒绝单个 Proposal |
| [**decideReviewTask**](GovernanceApi.md#decidereviewtask) | **POST** /api/v1/review-tasks/{id}/decision | 人工批准或拒绝 ReviewTask |
| [**finalizeBulkReviewBatch**](GovernanceApi.md#finalizebulkreviewbatch) | **POST** /api/v1/bulk-review-batches/{id}/finalize | 抽样通过后批准未抽样 Proposal 并冻结审核结果 |
| [**getBulkReviewBatch**](GovernanceApi.md#getbulkreviewbatch) | **GET** /api/v1/bulk-review-batches/{id} | 读取批量审核、Proposal 决策与固定 wave |
| [**getProposal**](GovernanceApi.md#getproposal) | **GET** /api/v1/proposals/{id} | 读取 Proposal、Operation 与冲突 |
| [**listBulkReviewAuditEvents**](GovernanceApi.md#listbulkreviewauditevents) | **GET** /api/v1/bulk-review-batches/{id}/audit-events | 查询批量审核、决策、暂停和 wave Apply 审计 |
| [**listReviewTasks**](GovernanceApi.md#listreviewtasks) | **GET** /api/v1/review-tasks | 人工审核队列 |
| [**mergeProposalToWorkingDocument**](GovernanceApi.md#mergeproposaltoworkingdocumentoperation) | **POST** /api/v1/proposals/{id}/merge-to-working-document | 以 sequence CAS 将已验证的 Proposal Yjs delta 合并到工作副本 |
| [**pauseBulkReviewBatch**](GovernanceApi.md#pausebulkreviewbatch) | **POST** /api/v1/bulk-review-batches/{id}/pause | 暂停后续 Proposal Apply |
| [**previewProposal**](GovernanceApi.md#previewproposal) | **GET** /api/v1/proposals/{id}/preview | 无权威写入地预览 Base、Current 与 Proposed |
| [**resolveMergeConflict**](GovernanceApi.md#resolvemergeconflictoperation) | **POST** /api/v1/proposals/{id}/conflicts/{conflict_id}/resolution | 记录单个 MergeConflict 的人工决议 |
| [**resumeBulkReviewBatch**](GovernanceApi.md#resumebulkreviewbatch) | **POST** /api/v1/bulk-review-batches/{id}/resume | 恢复后续 Proposal Apply |
| [**rollbackChangeBatch**](GovernanceApi.md#rollbackchangebatch) | **POST** /api/v1/change-batches/{id}/rollback | 以新版本补偿回滚 ChangeBatch |
| [**submitProposal**](GovernanceApi.md#submitproposal) | **POST** /api/v1/proposals/{id}/submit | 提交并执行风险策略 |



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

