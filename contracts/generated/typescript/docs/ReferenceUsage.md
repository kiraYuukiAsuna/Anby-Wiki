
# ReferenceUsage

Entity/Claim/Citation 在页面 Current Revision 中的一处引用位置

## Properties

Name | Type
------------ | -------------
`pageId` | string
`pageTitle` | string
`revisionId` | string
`blockId` | string
`nodeId` | string
`mentionText` | string
`claimId` | string

## Example

```typescript
import type { ReferenceUsage } from ''

// TODO: Update the object below with actual values
const example = {
  "pageId": null,
  "pageTitle": null,
  "revisionId": null,
  "blockId": null,
  "nodeId": null,
  "mentionText": null,
  "claimId": null,
} satisfies ReferenceUsage

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ReferenceUsage
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


