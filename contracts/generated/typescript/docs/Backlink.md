
# Backlink

一条反向链接：指向目标页的已解析引用来源

## Properties

Name | Type
------------ | -------------
`sourcePageId` | string
`sourceTitle` | string
`sourceBlockId` | string
`displayText` | string

## Example

```typescript
import type { Backlink } from ''

// TODO: Update the object below with actual values
const example = {
  "sourcePageId": null,
  "sourceTitle": Anby Demara,
  "sourceBlockId": null,
  "displayText": null,
} satisfies Backlink

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as Backlink
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


