
# EntityMergeResult


## Properties

Name | Type
------------ | -------------
`id` | string
`sourceEntityId` | string
`targetEntityId` | string
`actorId` | string
`status` | string
`reason` | string
`createdAt` | Date
`idempotent` | boolean
`labelMappings` | [Array&lt;EntityMergeLabelMapping&gt;](EntityMergeLabelMapping.md)
`claimMappings` | [Array&lt;EntityMergeClaimMapping&gt;](EntityMergeClaimMapping.md)

## Example

```typescript
import type { EntityMergeResult } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "sourceEntityId": null,
  "targetEntityId": null,
  "actorId": null,
  "status": null,
  "reason": null,
  "createdAt": null,
  "idempotent": null,
  "labelMappings": null,
  "claimMappings": null,
} satisfies EntityMergeResult

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as EntityMergeResult
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


