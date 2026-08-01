
# DatasetView


## Properties

Name | Type
------------ | -------------
`id` | string
`datasetId` | string
`viewType` | string
`name` | string
`config` | [DatasetViewConfig](DatasetViewConfig.md)
`createdBy` | string
`createdAt` | Date

## Example

```typescript
import type { DatasetView } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "datasetId": null,
  "viewType": null,
  "name": null,
  "config": null,
  "createdBy": null,
  "createdAt": null,
} satisfies DatasetView

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as DatasetView
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


