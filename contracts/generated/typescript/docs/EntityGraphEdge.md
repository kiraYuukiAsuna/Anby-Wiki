
# EntityGraphEdge


## Properties

Name | Type
------------ | -------------
`claimId` | string
`subjectEntityId` | string
`targetEntityId` | string
`propertyId` | string
`propertyKey` | string
`propertyName` | string
`rank` | string
`verificationStatus` | string
`claimCreatedAt` | Date
`projectedAt` | Date

## Example

```typescript
import type { EntityGraphEdge } from ''

// TODO: Update the object below with actual values
const example = {
  "claimId": null,
  "subjectEntityId": null,
  "targetEntityId": null,
  "propertyId": null,
  "propertyKey": null,
  "propertyName": null,
  "rank": null,
  "verificationStatus": null,
  "claimCreatedAt": null,
  "projectedAt": null,
} satisfies EntityGraphEdge

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as EntityGraphEdge
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


