
# ImportPlan


## Properties

Name | Type
------------ | -------------
`schemaVersion` | number
`sourceVersionId` | string
`profile` | [ImportPlanProfile](ImportPlanProfile.md)
`routes` | [Array&lt;ImportPageRoute&gt;](ImportPageRoute.md)
`qualityScore` | number
`promptInjectionDetected` | boolean

## Example

```typescript
import type { ImportPlan } from ''

// TODO: Update the object below with actual values
const example = {
  "schemaVersion": null,
  "sourceVersionId": null,
  "profile": null,
  "routes": null,
  "qualityScore": null,
  "promptInjectionDetected": null,
} satisfies ImportPlan

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportPlan
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


