
# ImportSafetyDecision


## Properties

Name | Type
------------ | -------------
`schemaVersion` | number
`sourceVersionId` | string
`extractionId` | string
`decision` | string
`reason` | string
`reviewedBy` | string
`reviewedAt` | Date

## Example

```typescript
import type { ImportSafetyDecision } from ''

// TODO: Update the object below with actual values
const example = {
  "schemaVersion": null,
  "sourceVersionId": null,
  "extractionId": null,
  "decision": null,
  "reason": null,
  "reviewedBy": null,
  "reviewedAt": null,
} satisfies ImportSafetyDecision

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportSafetyDecision
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


