
# ImportJob


## Properties

Name | Type
------------ | -------------
`id` | string
`jobType` | string
`status` | string
`initiatedBy` | string
`idempotencyKey` | string
`config` | { [key: string]: any; }
`sourceVersionId` | string
`proposalId` | string
`currentStage` | string
`progress` | number
`error` | { [key: string]: any; }
`createdAt` | Date
`startedAt` | Date
`finishedAt` | Date
`updatedAt` | Date

## Example

```typescript
import type { ImportJob } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "jobType": null,
  "status": null,
  "initiatedBy": null,
  "idempotencyKey": null,
  "config": null,
  "sourceVersionId": null,
  "proposalId": null,
  "currentStage": null,
  "progress": null,
  "error": null,
  "createdAt": null,
  "startedAt": null,
  "finishedAt": null,
  "updatedAt": null,
} satisfies ImportJob

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportJob
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


