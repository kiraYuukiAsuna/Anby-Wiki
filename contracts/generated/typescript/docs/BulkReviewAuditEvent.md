
# BulkReviewAuditEvent


## Properties

Name | Type
------------ | -------------
`id` | string
`batchId` | string
`actorId` | string
`eventType` | string
`proposalId` | string
`wave` | number
`payload` | { [key: string]: any; }
`createdAt` | Date

## Example

```typescript
import type { BulkReviewAuditEvent } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "batchId": null,
  "actorId": null,
  "eventType": null,
  "proposalId": null,
  "wave": null,
  "payload": null,
  "createdAt": null,
} satisfies BulkReviewAuditEvent

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as BulkReviewAuditEvent
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


