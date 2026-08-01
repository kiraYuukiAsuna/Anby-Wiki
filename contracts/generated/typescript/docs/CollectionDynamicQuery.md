
# CollectionDynamicQuery

Dynamic Collection v1 受限查询；不接受 SQL 或 AST 扫描表达式。

## Properties

Name | Type
------------ | -------------
`version` | number
`memberType` | string
`text` | string
`namespace` | string
`entityType` | string
`property` | string

## Example

```typescript
import type { CollectionDynamicQuery } from ''

// TODO: Update the object below with actual values
const example = {
  "version": null,
  "memberType": null,
  "text": null,
  "namespace": null,
  "entityType": null,
  "property": null,
} satisfies CollectionDynamicQuery

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CollectionDynamicQuery
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


