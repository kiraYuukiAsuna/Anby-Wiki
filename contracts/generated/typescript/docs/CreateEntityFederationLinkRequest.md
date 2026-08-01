
# CreateEntityFederationLinkRequest


## Properties

Name | Type
------------ | -------------
`remoteWikiId` | string
`remoteEntityId` | string
`remoteCanonicalKey` | string
`remoteLabel` | string
`relationType` | string
`verificationStatus` | string
`metadata` | { [key: string]: any; }

## Example

```typescript
import type { CreateEntityFederationLinkRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "remoteWikiId": null,
  "remoteEntityId": null,
  "remoteCanonicalKey": null,
  "remoteLabel": null,
  "relationType": null,
  "verificationStatus": null,
  "metadata": null,
} satisfies CreateEntityFederationLinkRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CreateEntityFederationLinkRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


