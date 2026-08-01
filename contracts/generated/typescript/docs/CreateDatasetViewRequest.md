
# CreateDatasetViewRequest


## Properties

Name | Type
------------ | -------------
`viewType` | string
`name` | string
`config` | [DatasetViewConfig](DatasetViewConfig.md)

## Example

```typescript
import type { CreateDatasetViewRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "viewType": null,
  "name": null,
  "config": null,
} satisfies CreateDatasetViewRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CreateDatasetViewRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


