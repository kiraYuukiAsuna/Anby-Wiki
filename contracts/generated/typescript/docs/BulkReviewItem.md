
# BulkReviewItem


## Properties

Name | Type
------------ | -------------
`batchId` | string
`proposalId` | string
`position` | number
`wave` | number
`selectedForReview` | boolean
`decision` | string
`decisionReason` | string
`reviewedBy` | string
`reviewedAt` | Date
`applyStatus` | string
`changeBatchId` | string
`applyErrorCode` | string
`appliedAt` | Date

## Example

```typescript
import type { BulkReviewItem } from ''

// TODO: Update the object below with actual values
const example = {
  "batchId": null,
  "proposalId": null,
  "position": null,
  "wave": null,
  "selectedForReview": null,
  "decision": null,
  "decisionReason": null,
  "reviewedBy": null,
  "reviewedAt": null,
  "applyStatus": null,
  "changeBatchId": null,
  "applyErrorCode": null,
  "appliedAt": null,
} satisfies BulkReviewItem

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as BulkReviewItem
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


