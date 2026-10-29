
# AuthSession


## Properties

Name | Type
------------ | -------------
`actorId` | string
`wikiId` | string
`homeWikiId` | string
`wikiName` | string
`mainNamespaceId` | string
`actorType` | string
`displayName` | string
`method` | string
`wikis` | [Array&lt;AuthWikiMembership&gt;](AuthWikiMembership.md)

## Example

```typescript
import type { AuthSession } from ''

// TODO: Update the object below with actual values
const example = {
  "actorId": null,
  "wikiId": null,
  "homeWikiId": null,
  "wikiName": null,
  "mainNamespaceId": null,
  "actorType": null,
  "displayName": null,
  "method": null,
  "wikis": null,
} satisfies AuthSession

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AuthSession
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


