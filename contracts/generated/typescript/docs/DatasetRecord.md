
# DatasetRecord


## Properties

Name | Type
------------ | -------------
`id` | string
`datasetId` | string
`entityId` | string
`values` | { [key: string]: any; }
`createdAt` | Date
`updatedAt` | Date

## Example

```typescript
import type { DatasetRecord } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "datasetId": null,
  "entityId": null,
  "values": null,
  "createdAt": null,
  "updatedAt": null,
} satisfies DatasetRecord

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as DatasetRecord
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


