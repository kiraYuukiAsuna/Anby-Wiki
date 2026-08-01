
# EntityFederationLink


## Properties

Name | Type
------------ | -------------
`id` | string
`wikiId` | string
`localEntityId` | string
`localCanonicalKey` | string
`localLabel` | string
`localStatus` | string
`remoteWikiId` | string
`remoteWikiKey` | string
`remoteWikiName` | string
`remoteWikiStatus` | string
`remoteTrustLevel` | string
`remoteEntityId` | string
`remoteCanonicalKey` | string
`remoteLabel` | string
`remoteEntityUrl` | string
`relationType` | string
`verificationStatus` | string
`status` | string
`metadata` | { [key: string]: any; }
`createdBy` | string
`updatedBy` | string
`createdAt` | Date
`updatedAt` | Date

## Example

```typescript
import type { EntityFederationLink } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "wikiId": null,
  "localEntityId": null,
  "localCanonicalKey": null,
  "localLabel": null,
  "localStatus": null,
  "remoteWikiId": null,
  "remoteWikiKey": null,
  "remoteWikiName": null,
  "remoteWikiStatus": null,
  "remoteTrustLevel": null,
  "remoteEntityId": null,
  "remoteCanonicalKey": null,
  "remoteLabel": null,
  "remoteEntityUrl": null,
  "relationType": null,
  "verificationStatus": null,
  "status": null,
  "metadata": null,
  "createdBy": null,
  "updatedBy": null,
  "createdAt": null,
  "updatedAt": null,
} satisfies EntityFederationLink

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as EntityFederationLink
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


