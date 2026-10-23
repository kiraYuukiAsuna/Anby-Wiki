
# ImportEntityResolution


## Properties

Name | Type
------------ | -------------
`candidateId` | string
`outcome` | string
`entityId` | string
`plannedEntityId` | string
`score` | number
`reason` | string
`alternatives` | [Array&lt;ImportEntityAlternative&gt;](ImportEntityAlternative.md)

## Example

```typescript
import type { ImportEntityResolution } from ''

// TODO: Update the object below with actual values
const example = {
  "candidateId": null,
  "outcome": null,
  "entityId": null,
  "plannedEntityId": null,
  "score": null,
  "reason": null,
  "alternatives": null,
} satisfies ImportEntityResolution

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportEntityResolution
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


