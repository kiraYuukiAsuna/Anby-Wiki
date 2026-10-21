
# PublicDatasetForm


## Properties

Name | Type
------------ | -------------
`id` | string
`datasetId` | string
`name` | string
`fields` | [Array&lt;PublicDatasetFormField&gt;](PublicDatasetFormField.md)
`submitLabel` | string
`successMessage` | string

## Example

```typescript
import type { PublicDatasetForm } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "datasetId": null,
  "name": null,
  "fields": null,
  "submitLabel": null,
  "successMessage": null,
} satisfies PublicDatasetForm

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PublicDatasetForm
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


