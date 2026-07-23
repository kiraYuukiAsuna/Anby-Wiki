
# PropertySummary


## Properties

Name | Type
------------ | -------------
`id` | string
`propertyKey` | string
`name` | string
`valueType` | string
`isMultivalued` | boolean
`schema` | { [key: string]: any; }

## Example

```typescript
import type { PropertySummary } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "propertyKey": null,
  "name": null,
  "valueType": null,
  "isMultivalued": null,
  "schema": null,
} satisfies PropertySummary

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PropertySummary
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


