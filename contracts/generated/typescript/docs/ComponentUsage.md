
# ComponentUsage


## Properties

Name | Type
------------ | -------------
`pageId` | string
`pageTitle` | string
`revisionId` | string
`blockId` | string
`componentVersion` | number
`entityId` | string

## Example

```typescript
import type { ComponentUsage } from ''

// TODO: Update the object below with actual values
const example = {
  "pageId": null,
  "pageTitle": null,
  "revisionId": null,
  "blockId": null,
  "componentVersion": null,
  "entityId": null,
} satisfies ComponentUsage

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ComponentUsage
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


