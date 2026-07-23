
# Page


## Properties

Name | Type
------------ | -------------
`id` | string
`wikiId` | string
`namespaceId` | string
`normalizedTitle` | string
`displayTitle` | string
`language` | string
`contentModel` | string
`status` | string
`currentRevisionId` | string
`createdBy` | string
`createdAt` | Date
`updatedAt` | Date

## Example

```typescript
import type { Page } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "wikiId": null,
  "namespaceId": null,
  "normalizedTitle": anby demara,
  "displayTitle": Anby Demara,
  "language": zh-Hans,
  "contentModel": block-v1,
  "status": active,
  "currentRevisionId": null,
  "createdBy": null,
  "createdAt": null,
  "updatedAt": null,
} satisfies Page

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as Page
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


