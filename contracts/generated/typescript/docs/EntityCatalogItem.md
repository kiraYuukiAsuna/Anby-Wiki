
# EntityCatalogItem


## Properties

Name | Type
------------ | -------------
`id` | string
`wikiId` | string
`canonicalKey` | string
`status` | string
`mergedIntoEntityId` | string
`entityType` | [EntityTypeSummary](EntityTypeSummary.md)
`displayLabel` | string
`displayLanguage` | string
`description` | string
`labelCount` | number
`aliasCount` | number
`claimCount` | number
`pageCount` | number
`createdAt` | Date
`updatedAt` | Date

## Example

```typescript
import type { EntityCatalogItem } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "wikiId": null,
  "canonicalKey": null,
  "status": null,
  "mergedIntoEntityId": null,
  "entityType": null,
  "displayLabel": null,
  "displayLanguage": null,
  "description": null,
  "labelCount": null,
  "aliasCount": null,
  "claimCount": null,
  "pageCount": null,
  "createdAt": null,
  "updatedAt": null,
} satisfies EntityCatalogItem

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as EntityCatalogItem
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


