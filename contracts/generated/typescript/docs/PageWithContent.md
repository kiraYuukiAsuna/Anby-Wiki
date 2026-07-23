
# PageWithContent

阅读端点响应：落地页元信息 + 当前 Revision 内容（渲染直连 ContentSnapshot）

## Properties

Name | Type
------------ | -------------
`page` | [Page](Page.md)
`content` | [PageContent](PageContent.md)
`viaAlias` | boolean
`aliasTitle` | string
`redirect` | [PageWithContentRedirect](PageWithContentRedirect.md)

## Example

```typescript
import type { PageWithContent } from ''

// TODO: Update the object below with actual values
const example = {
  "page": null,
  "content": null,
  "viaAlias": null,
  "aliasTitle": null,
  "redirect": null,
} satisfies PageWithContent

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PageWithContent
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


