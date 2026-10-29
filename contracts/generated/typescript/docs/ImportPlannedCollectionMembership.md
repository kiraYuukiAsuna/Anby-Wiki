
# ImportPlannedCollectionMembership


## Properties

Name | Type
------------ | -------------
`collectionId` | string
`title` | string
`reason` | string
`confidence` | number
`evidence` | [Array&lt;ImportPlanEvidence&gt;](ImportPlanEvidence.md)

## Example

```typescript
import type { ImportPlannedCollectionMembership } from ''

// TODO: Update the object below with actual values
const example = {
  "collectionId": null,
  "title": null,
  "reason": null,
  "confidence": null,
  "evidence": null,
} satisfies ImportPlannedCollectionMembership

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportPlannedCollectionMembership
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


