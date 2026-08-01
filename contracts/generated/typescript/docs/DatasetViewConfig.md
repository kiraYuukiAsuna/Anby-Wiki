
# DatasetViewConfig


## Properties

Name | Type
------------ | -------------
`columns` | Set&lt;string&gt;
`filter` | [DatasetViewFilter](DatasetViewFilter.md)
`sort` | [DatasetViewSort](DatasetViewSort.md)
`groupBy` | string

## Example

```typescript
import type { DatasetViewConfig } from ''

// TODO: Update the object below with actual values
const example = {
  "columns": null,
  "filter": null,
  "sort": null,
  "groupBy": null,
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


