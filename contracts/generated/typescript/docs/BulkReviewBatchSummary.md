
# BulkReviewBatchSummary


## Properties

Name | Type
------------ | -------------
`id` | string
`wikiId` | string
`createdBy` | string
`status` | string
`samplingMode` | string
`samplePercent` | number
`forceFullReason` | string
`waveSize` | number
`currentWave` | number
`itemCount` | number
`pendingDecisions` | number
`approvedCount` | number
`rejectedCount` | number
`appliedCount` | number
`failedCount` | number
`createdAt` | Date
`completedAt` | Date

## Example

```typescript
import type { BulkReviewBatchSummary } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "wikiId": null,
  "createdBy": null,
  "status": null,
  "samplingMode": null,
  "samplePercent": null,
  "forceFullReason": null,
  "waveSize": null,
  "currentWave": null,
  "itemCount": null,
  "pendingDecisions": null,
  "approvedCount": null,
  "rejectedCount": null,
  "appliedCount": null,
  "failedCount": null,
  "createdAt": null,
  "completedAt": null,
} satisfies BulkReviewBatchSummary

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as BulkReviewBatchSummary
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


