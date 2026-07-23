
# MergeProposalToWorkingDocumentResult


## Properties

Name | Type
------------ | -------------
`proposalId` | string
`changeBatchId` | string
`documentId` | string
`sequence` | number
`idempotent` | boolean

## Example

```typescript
import type { MergeProposalToWorkingDocumentResult } from ''

// TODO: Update the object below with actual values
const example = {
  "proposalId": null,
  "changeBatchId": null,
  "documentId": null,
  "sequence": null,
  "idempotent": null,
} satisfies MergeProposalToWorkingDocumentResult

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as MergeProposalToWorkingDocumentResult
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


