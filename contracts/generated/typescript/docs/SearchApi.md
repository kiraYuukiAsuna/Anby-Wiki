# SearchApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getSearchCapabilities**](SearchApi.md#getsearchcapabilities) | **GET** /api/v1/search/capabilities | 获取搜索引擎能力 |
| [**searchPages**](SearchApi.md#searchpages) | **GET** /api/v1/pages/search | 搜索活页面 |



## getSearchCapabilities

> SearchCapabilities getSearchCapabilities()

获取搜索引擎能力

匿名可读。返回当前 SearchAdapter 的真实后端、可用排序模式和聚合维度； 未配置向量 Embedder 时不会宣称支持 hybrid / semantic。

### Example

```ts
import {
  Configuration,
  SearchApi,
} from '';
import type { GetSearchCapabilitiesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new SearchApi();

  try {
    const data = await api.getSearchCapabilities();
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

[**SearchCapabilities**](SearchCapabilities.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 当前搜索能力 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## searchPages

> PageSearchResults searchPages(q, namespace, language, entityType, fields, mode, semanticRatio, limit, offset)

搜索活页面

独立搜索引擎 SearchAdapter 的匿名查询端点（无需登录）。 搜索文档由 Outbox 驱动的 Current Revision 投影生成，覆盖标题、旧别名、 正文和主 Entity 文本；支持关键词、混合、纯语义搜索，以及字段、 命名空间、语言与 Entity 类型过滤和聚合。 PostgreSQL 仅作为可重建权威 staging 与开发回退；回退环境只支持 keyword。 高亮使用 [[ 与 ]] 标记，调用方必须按纯文本处理，不能作为 HTML 注入。 q 为空（trim 后）返回空列表；limit 缺省 10、最大 50。

### Example

```ts
import {
  Configuration,
  SearchApi,
} from '';
import type { SearchPagesRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new SearchApi();

  const body = {
    // string | 查询词（空白裁剪后为空则返回空列表） (optional)
    q: anby,
    // string | 命名空间 key；缺省不过滤 (optional)
    namespace: main,
    // string | 页面语言精确过滤；缺省不过滤 (optional)
    language: zh-Hans,
    // string | 页面主 Entity 类型 key 精确过滤；缺省不过滤 (optional)
    entityType: character,
    // Array<'title' | 'alias' | 'body' | 'entity'> | 搜索字段；缺省搜索全部字段，可重复传递 (optional)
    fields: ...,
    // 'keyword' | 'hybrid' | 'semantic' | 排序模式；hybrid / semantic 仅在能力端点声明可用时使用 (optional)
    mode: mode_example,
    // number | hybrid 模式下语义结果权重；缺省 0.5 (optional)
    semanticRatio: 1.2,
    // number | 返回条数上限（缺省 10，最大 50，超出截断） (optional)
    limit: 56,
    // number | 分页偏移量（缺省 0） (optional)
    offset: 56,
  } satisfies SearchPagesRequest;

  try {
    const data = await api.searchPages(body);
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
| **q** | `string` | 查询词（空白裁剪后为空则返回空列表） | [Optional] [Defaults to `undefined`] |
| **namespace** | `string` | 命名空间 key；缺省不过滤 | [Optional] [Defaults to `undefined`] |
| **language** | `string` | 页面语言精确过滤；缺省不过滤 | [Optional] [Defaults to `undefined`] |
| **entityType** | `string` | 页面主 Entity 类型 key 精确过滤；缺省不过滤 | [Optional] [Defaults to `undefined`] |
| **fields** | `title`, `alias`, `body`, `entity` | 搜索字段；缺省搜索全部字段，可重复传递 | [Optional] [Enum: title, alias, body, entity] |
| **mode** | `keyword`, `hybrid`, `semantic` | 排序模式；hybrid / semantic 仅在能力端点声明可用时使用 | [Optional] [Defaults to `&#39;keyword&#39;`] [Enum: keyword, hybrid, semantic] |
| **semanticRatio** | `number` | hybrid 模式下语义结果权重；缺省 0.5 | [Optional] [Defaults to `0.5`] |
| **limit** | `number` | 返回条数上限（缺省 10，最大 50，超出截断） | [Optional] [Defaults to `10`] |
| **offset** | `number` | 分页偏移量（缺省 0） | [Optional] [Defaults to `0`] |

### Return type

[**PageSearchResults**](PageSearchResults.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 搜索结果与命中总数 |  * X-Request-ID -  <br>  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **500** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

