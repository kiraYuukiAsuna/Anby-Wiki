
# ImportPlannedBlock


## Properties

Name | Type
------------ | -------------
`type` | string
`mode` | string
`targetBlockId` | string
`text` | string
`level` | number
`isToggleable` | boolean
`checked` | boolean
`kind` | string
`icon` | string
`color` | string
`language` | string
`caption` | string
`expression` | string
`items` | Array&lt;string&gt;
`rows` | Array&lt;Array&lt;string&gt;&gt;
`hasColumnHeader` | boolean
`hasRowHeader` | boolean
`children` | [Array&lt;ImportPlannedBlock&gt;](ImportPlannedBlock.md)
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
  "isToggleable": null,
  "checked": null,
  "kind": null,
  "icon": null,
  "color": null,
  "language": null,
  "caption": null,
  "expression": null,
  "items": null,
  "rows": null,
  "hasColumnHeader": null,
  "hasRowHeader": null,
  "children": null,
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


