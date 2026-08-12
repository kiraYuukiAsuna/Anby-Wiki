
# ApplyProposalResult


## Properties

Name | Type
------------ | -------------
`proposalId` | string
`changeBatchId` | string
`revisionId` | string
`revisionIds` | Array&lt;string&gt;
`pageIds` | Array&lt;string&gt;
`entityIds` | Array&lt;string&gt;
`entityMergeIds` | Array&lt;string&gt;
`collectionIds` | Array&lt;string&gt;
`claimIds` | Array&lt;string&gt;
`idempotent` | boolean

## Example

```typescript
import type { ApplyProposalResult } from ''

// TODO: Update the object below with actual values
const example = {
  "proposalId": null,
  "changeBatchId": null,
  "revisionId": null,
  "revisionIds": null,
  "pageIds": null,
  "entityIds": null,
  "entityMergeIds": null,
  "collectionIds": null,
  "claimIds": null,
  "idempotent": null,
} satisfies ApplyProposalResult

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ApplyProposalResult
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


