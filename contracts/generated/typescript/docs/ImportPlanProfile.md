
# ImportPlanProfile


## Properties

Name | Type
------------ | -------------
`title` | string
`summary` | string
`language` | string
`useful` | boolean
`subjects` | [Array&lt;ImportPlanSubject&gt;](ImportPlanSubject.md)

## Example

```typescript
import type { ImportPlanProfile } from ''

// TODO: Update the object below with actual values
const example = {
  "title": null,
  "summary": null,
  "language": null,
  "useful": null,
  "subjects": null,
} satisfies ImportPlanProfile

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportPlanProfile
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


