
# ImportJobDetail


## Properties

Name | Type
------------ | -------------
`job` | [ImportJob](ImportJob.md)
`runs` | [Array&lt;ImportRun&gt;](ImportRun.md)
`stages` | [Array&lt;ImportStageRun&gt;](ImportStageRun.md)

## Example

```typescript
import type { ImportJobDetail } from ''

// TODO: Update the object below with actual values
const example = {
  "job": null,
  "runs": null,
  "stages": null,
} satisfies ImportJobDetail

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportJobDetail
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


