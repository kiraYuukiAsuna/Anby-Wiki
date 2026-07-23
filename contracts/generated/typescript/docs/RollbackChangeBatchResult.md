
# RollbackChangeBatchResult


## Properties

Name | Type
------------ | -------------
`changeBatchId` | string
`revisionIds` | Array&lt;string&gt;
`compensationClaimIds` | Array&lt;string&gt;
`idempotent` | boolean

## Example

```typescript
import type { RollbackChangeBatchResult } from ''

// TODO: Update the object below with actual values
const example = {
  "changeBatchId": null,
  "revisionIds": null,
  "compensationClaimIds": null,
  "idempotent": null,
} satisfies RollbackChangeBatchResult

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as RollbackChangeBatchResult
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


