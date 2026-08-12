
# ImportPlannedBlock


## Properties

Name | Type
------------ | -------------
`type` | string
`mode` | string
`targetBlockId` | string
`text` | string
`level` | number
`items` | Array&lt;string&gt;
`evidence` | [Array&lt;ImportPlanEvidence&gt;](ImportPlanEvidence.md)

## Example

```typescript
import type { ImportPlannedBlock } from ''

// TODO: Update the object below with actual values
const example = {
  "type": null,
  "mode": null,
  "targetBlockId": null,
  "text": null,
  "level": null,
  "items": null,
  "evidence": null,
} satisfies ImportPlannedBlock

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportPlannedBlock
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


