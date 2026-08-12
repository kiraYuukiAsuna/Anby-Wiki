
# PageCollection


## Properties

Name | Type
------------ | -------------
`id` | string
`wikiId` | string
`collectionType` | string
`title` | string
`descriptionPageId` | string
`query` | [CollectionQuery](CollectionQuery.md)
`createdAt` | Date
`updatedAt` | Date
`membershipSource` | string

## Example

```typescript
import type { PageCollection } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "wikiId": null,
  "collectionType": null,
  "title": null,
  "descriptionPageId": null,
  "query": null,
  "createdAt": null,
  "updatedAt": null,
  "membershipSource": null,
} satisfies PageCollection

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PageCollection
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


