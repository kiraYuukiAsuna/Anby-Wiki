
# AITrustProfile


## Properties

Name | Type
------------ | -------------
`wikiId` | string
`actorId` | string
`actorType` | string
`actorDisplayName` | string
`trustLevel` | string
`requiredSamplePercent` | number
`configured` | boolean
`updatedBy` | string
`createdAt` | Date
`updatedAt` | Date

## Example

```typescript
import type { AITrustProfile } from ''

// TODO: Update the object below with actual values
const example = {
  "wikiId": null,
  "actorId": null,
  "actorType": null,
  "actorDisplayName": null,
  "trustLevel": null,
  "requiredSamplePercent": null,
  "configured": null,
  "updatedBy": null,
  "createdAt": null,
  "updatedAt": null,
} satisfies AITrustProfile

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AITrustProfile
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


