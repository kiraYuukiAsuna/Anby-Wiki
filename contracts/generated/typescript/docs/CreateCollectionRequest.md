
# CreateCollectionRequest


## Properties

Name | Type
------------ | -------------
`collectionType` | string
`title` | string
`descriptionPageId` | string
`query` | [CollectionQuery](CollectionQuery.md)

## Example

```typescript
import type { CreateCollectionRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "collectionType": null,
  "title": null,
  "descriptionPageId": null,
  "query": null,
} satisfies CreateCollectionRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CreateCollectionRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


