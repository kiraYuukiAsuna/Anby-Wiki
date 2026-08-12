
# ImportPageRoute


## Properties

Name | Type
------------ | -------------
`action` | string
`title` | string
`pageId` | string
`reason` | string
`confidence` | number
`relatedTo` | Array&lt;string&gt;
`evidence` | [Array&lt;ImportPlanEvidence&gt;](ImportPlanEvidence.md)
`blocks` | [Array&lt;ImportPlannedBlock&gt;](ImportPlannedBlock.md)

## Example

```typescript
import type { ImportPageRoute } from ''

// TODO: Update the object below with actual values
const example = {
  "action": null,
  "title": null,
  "pageId": null,
  "reason": null,
  "confidence": null,
  "relatedTo": null,
  "evidence": null,
  "blocks": null,
} satisfies ImportPageRoute

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportPageRoute
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


