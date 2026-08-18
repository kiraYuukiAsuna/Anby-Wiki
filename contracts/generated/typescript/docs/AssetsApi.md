# AssetsApi

All URIs are relative to *http://localhost:3000*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getAssetRevision**](AssetsApi.md#getassetrevision) | **GET** /api/v1/assets/revisions/{revision_id} | 读取不可变资产版本元数据 |
| [**getAssetRevisionContent**](AssetsApi.md#getassetrevisioncontent) | **GET** /api/v1/assets/revisions/{revision_id}/content | 流式读取不可变资产版本内容 |
| [**listAssets**](AssetsApi.md#listassets) | **GET** /api/v1/assets | 列出当前站点的媒体资产 |
| [**uploadAsset**](AssetsApi.md#uploadasset) | **POST** /api/v1/assets | 上传媒体资产并创建不可变版本 |



## getAssetRevision

> AssetRevision getAssetRevision(revisionId)

读取不可变资产版本元数据

### Example

```ts
import {
  Configuration,
  AssetsApi,
} from '';
import type { GetAssetRevisionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AssetsApi();

  const body = {
    // string | 不可变 AssetRevision ID
    revisionId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetAssetRevisionRequest;

  try {
    const data = await api.getAssetRevision(body);
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
| **revisionId** | `string` | 不可变 AssetRevision ID | [Defaults to `undefined`] |

### Return type

[**AssetRevision**](AssetRevision.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 资产版本元数据 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getAssetRevisionContent

> Blob getAssetRevisionContent(revisionId)

流式读取不可变资产版本内容

### Example

```ts
import {
  Configuration,
  AssetsApi,
} from '';
import type { GetAssetRevisionContentRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AssetsApi();

  const body = {
    // string | 不可变 AssetRevision ID
    revisionId: 38400000-8cf0-11bd-b23e-10b96e4ef00d,
  } satisfies GetAssetRevisionContentRequest;

  try {
    const data = await api.getAssetRevisionContent(body);
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
| **revisionId** | `string` | 不可变 AssetRevision ID | [Defaults to `undefined`] |

### Return type

**Blob**

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/octet-stream`, `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 带 immutable 缓存头的原始资产内容 |  * ETag -  <br>  * Cache-Control -  <br>  |
| **304** | ETag 未变化 |  -  |
| **400** | 请求格式错误 |  -  |
| **404** | 资源不存在 |  -  |
| **503** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## listAssets

> AssetListPage listAssets(cursor, pageSize, kind)

列出当前站点的媒体资产

### Example

```ts
import {
  Configuration,
  AssetsApi,
} from '';
import type { ListAssetsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AssetsApi();

  const body = {
    // string | 上一页响应返回的 next_cursor；首页不传 (optional)
    cursor: cursor_example,
    // number | 每页条数，默认 20，最大 100 (optional)
    pageSize: 56,
    // 'image' | 'video' | 'other' (optional)
    kind: kind_example,
  } satisfies ListAssetsRequest;

  try {
    const data = await api.listAssets(body);
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
| **kind** | `image`, `video`, `other` |  | [Optional] [Defaults to `undefined`] [Enum: image, video, other] |

### Return type

[**AssetListPage**](AssetListPage.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 资产目录游标分页结果 |  -  |
| **400** | 请求格式错误 |  -  |
| **503** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## uploadAsset

> Asset uploadAsset(file, name)

上传媒体资产并创建不可变版本

### Example

```ts
import {
  Configuration,
  AssetsApi,
} from '';
import type { UploadAssetRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({
    // Configure HTTP bearer authorization: cliBearer
    accessToken: "YOUR BEARER TOKEN",
    // To configure API key authorization: sessionCookie
    apiKey: "YOUR API KEY",
  });
  const api = new AssetsApi(config);

  const body = {
    // Blob
    file: BINARY_DATA_HERE,
    // string (optional)
    name: name_example,
  } satisfies UploadAssetRequest;

  try {
    const data = await api.uploadAsset(body);
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
| **file** | `Blob` |  | [Defaults to `undefined`] |
| **name** | `string` |  | [Optional] [Defaults to `undefined`] |

### Return type

[**Asset**](Asset.md)

### Authorization

[cliBearer](../README.md#cliBearer), [sessionCookie](../README.md#sessionCookie)

### HTTP request headers

- **Content-Type**: `multipart/form-data`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | 内容去重命中，返回既有资产版本 |  -  |
| **201** | 新资产或新版本 |  -  |
| **400** | 请求格式错误 |  -  |
| **401** | 未认证 |  -  |
| **403** | 已认证但无权限 |  -  |
| **413** | 请求格式错误 |  -  |
| **503** | 服务端内部错误 |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

