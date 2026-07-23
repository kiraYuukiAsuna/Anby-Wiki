
# RollbackResponse


## Properties

Name | Type
------------ | -------------
`id` | string
`pageId` | string
`parentRevisionId` | string
`contentSnapshotId` | string
`actorId` | string
`summary` | string
`isMinor` | boolean
`visibility` | string
`contentHash` | string
`schemaVersion` | number
`createdAt` | Date
`rolledBackTo` | string

## Example

```typescript
import type { RollbackResponse } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "pageId": null,
  "parentRevisionId": null,
  "contentSnapshotId": null,
  "actorId": null,
  "summary": null,
  "isMinor": null,
  "visibility": public,
  "contentHash": 3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb50adc8b7,
  "schemaVersion": 1,
  "createdAt": null,
  "rolledBackTo": null,
} satisfies RollbackResponse

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as RollbackResponse
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


