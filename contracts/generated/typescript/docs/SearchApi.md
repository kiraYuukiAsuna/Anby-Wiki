# SearchApi

All URIs are relative to *http://localhost:8000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**searchPages**](SearchApi.md#searchpages) | **GET** /api/v1/pages/search | 搜索活页面 |



## searchPages

> PageSearchResults searchPages(q, namespace, language, entityType, fields, limit, offset)

搜索活页面

PostgreSQL FTS SearchAdapter 的匿名查询端点（M7-T01，无需登录）。 搜索文档由 Outbox 驱动的 Current Revision 投影生成，覆盖标题、旧别名、 正文和主 Entity 文本；支持字段、命名空间、语言与 Entity 类型过滤。 高亮使用 [[ 与 ]] 标记，调用方必须按纯文本处理，不能作为 HTML 注入。 q 为空（trim 后）返回空列表；limit 缺省 10、最大 50。

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
    // string | 命名空间 key（缺省 main） (optional)
    namespace: main,
    // string | 页面语言精确过滤；缺省不过滤 (optional)
    language: zh-Hans,
    // string | 页面主 Entity 类型 key 精确过滤；缺省不过滤 (optional)
    entityType: character,
    // Array<'title' | 'alias' | 'body' | 'entity'> | 搜索字段；缺省搜索全部字段，可重复传递 (optional)
    fields: ...,
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
| **namespace** | `string` | 命名空间 key（缺省 main） | [Optional] [Defaults to `&#39;main&#39;`] |
| **language** | `string` | 页面语言精确过滤；缺省不过滤 | [Optional] [Defaults to `undefined`] |
| **entityType** | `string` | 页面主 Entity 类型 key 精确过滤；缺省不过滤 | [Optional] [Defaults to `undefined`] |
| **fields** | `title`, `alias`, `body`, `entity` | 搜索字段；缺省搜索全部字段，可重复传递 | [Optional] [Enum: title, alias, body, entity] |
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

