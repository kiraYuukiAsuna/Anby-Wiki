
# AuditEvent


## Properties

Name | Type
------------ | -------------
`id` | string
`actorId` | string
`actorDisplayName` | string
`eventType` | string
`aggregateType` | string
`aggregateId` | string
`changeBatchId` | string
`payload` | { [key: string]: any; }
`tags` | [Array&lt;ChangeTag&gt;](ChangeTag.md)
`createdAt` | Date

## Example

```typescript
import type { AuditEvent } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "actorId": null,
  "actorDisplayName": null,
  "eventType": null,
  "aggregateType": null,
  "aggregateId": null,
  "changeBatchId": null,
  "payload": null,
  "tags": null,
  "createdAt": null,
} satisfies AuditEvent

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AuditEvent
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


