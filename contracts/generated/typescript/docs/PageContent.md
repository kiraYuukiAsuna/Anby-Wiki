
# PageContent

已发布页面的当前 Revision 内容。delivery_mode=sections 时全文 AST/HTML 为 null，客户端按 PageSectionManifest 加载可重建分片。

## Properties

Name | Type
------------ | -------------
`revision` | [Revision](Revision.md)
`deliveryMode` | string
`sectionCount` | number
`astJson` | { [key: string]: any; }
`html` | string
`rendererVersion` | string

## Example

```typescript
import type { PageContent } from ''

// TODO: Update the object below with actual values
const example = {
  "revision": null,
  "deliveryMode": null,
  "sectionCount": null,
  "astJson": null,
  "html": null,
  "rendererVersion": v1,
} satisfies PageContent

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PageContent
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


