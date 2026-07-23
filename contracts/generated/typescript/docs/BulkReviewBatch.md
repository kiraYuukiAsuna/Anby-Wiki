
# BulkReviewBatch

独立批量审核聚合，不替代或合并 ChangeBatch

## Properties

Name | Type
------------ | -------------
`id` | string
`createdBy` | string
`status` | string
`samplingMode` | string
`samplePercent` | number
`forceFullReason` | string
`waveSize` | number
`currentWave` | number
`createdAt` | Date
`finalizedAt` | Date
`pausedAt` | Date
`completedAt` | Date
`items` | [Array&lt;BulkReviewItem&gt;](BulkReviewItem.md)

## Example

```typescript
import type { BulkReviewBatch } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "createdBy": null,
  "status": null,
  "samplingMode": null,
  "samplePercent": null,
  "forceFullReason": null,
  "waveSize": null,
  "currentWave": null,
  "createdAt": null,
  "finalizedAt": null,
  "pausedAt": null,
  "completedAt": null,
  "items": null,
} satisfies BulkReviewBatch

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as BulkReviewBatch
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


