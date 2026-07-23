
# EntityDetail


## Properties

Name | Type
------------ | -------------
`id` | string
`wikiId` | string
`canonicalKey` | string
`status` | string
`mergedIntoEntityId` | string
`entityType` | [EntityTypeSummary](EntityTypeSummary.md)
`labels` | [Array&lt;EntityLabel&gt;](EntityLabel.md)
`aliases` | [Array&lt;EntityAlias&gt;](EntityAlias.md)
`createdAt` | Date
`updatedAt` | Date

## Example

```typescript
import type { EntityDetail } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "wikiId": null,
  "canonicalKey": null,
  "status": null,
  "mergedIntoEntityId": null,
  "entityType": null,
  "labels": null,
  "aliases": null,
  "createdAt": null,
  "updatedAt": null,
} satisfies EntityDetail

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as EntityDetail
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


