
# CreateBulkReviewBatchRequest


## Properties

Name | Type
------------ | -------------
`proposalIds` | Set&lt;string&gt;
`samplePercent` | number
`forceFull` | boolean
`waveSize` | number

## Example

```typescript
import type { CreateBulkReviewBatchRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "proposalIds": null,
  "samplePercent": null,
  "forceFull": null,
  "waveSize": null,
} satisfies CreateBulkReviewBatchRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CreateBulkReviewBatchRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


