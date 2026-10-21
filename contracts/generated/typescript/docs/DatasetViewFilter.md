
# DatasetViewFilter

叶子条件使用 field/operator；组合条件使用 logic/conditions，服务端限制最多三层。

## Properties

Name | Type
------------ | -------------
`field` | string
`operator` | string
`value` | any
`logic` | string
`conditions` | [Array&lt;DatasetViewFilter&gt;](DatasetViewFilter.md)

## Example

```typescript
import type { DatasetViewFilter } from ''

// TODO: Update the object below with actual values
const example = {
  "field": null,
  "operator": null,
  "value": null,
  "logic": null,
  "conditions": null,
} satisfies DatasetViewFilter

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as DatasetViewFilter
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


