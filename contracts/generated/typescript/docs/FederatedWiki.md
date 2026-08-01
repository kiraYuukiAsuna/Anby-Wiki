
# FederatedWiki


## Properties

Name | Type
------------ | -------------
`id` | string
`wikiId` | string
`wikiKey` | string
`displayName` | string
`baseUrl` | string
`entityUrlTemplate` | string
`trustLevel` | string
`status` | string
`metadata` | { [key: string]: any; }
`createdBy` | string
`createdAt` | Date
`updatedAt` | Date

## Example

```typescript
import type { FederatedWiki } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "wikiId": null,
  "wikiKey": null,
  "displayName": null,
  "baseUrl": null,
  "entityUrlTemplate": null,
  "trustLevel": null,
  "status": null,
  "metadata": null,
  "createdBy": null,
  "createdAt": null,
  "updatedAt": null,
} satisfies FederatedWiki

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as FederatedWiki
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


