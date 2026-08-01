
# Dataset


## Properties

Name | Type
------------ | -------------
`id` | string
`wikiId` | string
`name` | string
`schema` | { [key: string]: any; }
`createdBy` | string
`createdAt` | Date

## Example

```typescript
import type { Dataset } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "wikiId": null,
  "name": null,
  "schema": null,
  "createdBy": null,
  "createdAt": null,
} satisfies Dataset

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as Dataset
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


