
# RollbackEntityMergeResult


## Properties

Name | Type
------------ | -------------
`mergeId` | string
`restoredEntityId` | string
`compensatedClaimIds` | Array&lt;string&gt;
`removedTargetLabels` | number
`idempotent` | boolean

## Example

```typescript
import type { RollbackEntityMergeResult } from ''

// TODO: Update the object below with actual values
const example = {
  "mergeId": null,
  "restoredEntityId": null,
  "compensatedClaimIds": null,
  "removedTargetLabels": null,
  "idempotent": null,
} satisfies RollbackEntityMergeResult

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as RollbackEntityMergeResult
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


