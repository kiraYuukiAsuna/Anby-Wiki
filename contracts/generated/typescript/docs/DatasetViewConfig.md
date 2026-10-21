
# DatasetViewConfig


## Properties

Name | Type
------------ | -------------
`columns` | Set&lt;string&gt;
`filter` | [DatasetViewFilter](DatasetViewFilter.md)
`sort` | [DatasetViewSort](DatasetViewSort.md)
`groupBy` | string
`viewMode` | string
`sorts` | [Array&lt;DatasetViewSort&gt;](DatasetViewSort.md)
`groups` | [Array&lt;DatasetViewGroup&gt;](DatasetViewGroup.md)
`titleField` | string
`descriptionField` | string
`coverField` | string
`dateField` | string
`endDateField` | string
`cardSize` | string
`formFields` | Set&lt;string&gt;
`_public` | boolean
`submitLabel` | string
`successMessage` | string
`viewIds` | Set&lt;string&gt;
`globalFilterFields` | Set&lt;string&gt;
`layoutColumns` | number

## Example

```typescript
import type { DatasetViewConfig } from ''

// TODO: Update the object below with actual values
const example = {
  "columns": null,
  "filter": null,
  "sort": null,
  "groupBy": null,
  "viewMode": null,
  "sorts": null,
  "groups": null,
  "titleField": null,
  "descriptionField": null,
  "coverField": null,
  "dateField": null,
  "endDateField": null,
  "cardSize": null,
  "formFields": null,
  "_public": null,
  "submitLabel": null,
  "successMessage": null,
  "viewIds": null,
  "globalFilterFields": null,
  "layoutColumns": null,
} satisfies DatasetViewConfig

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as DatasetViewConfig
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


