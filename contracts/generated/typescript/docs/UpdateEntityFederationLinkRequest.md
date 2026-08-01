
# UpdateEntityFederationLinkRequest


## Properties

Name | Type
------------ | -------------
`remoteCanonicalKey` | string
`remoteLabel` | string
`relationType` | string
`verificationStatus` | string
`status` | string
`metadata` | { [key: string]: any; }

## Example

```typescript
import type { UpdateEntityFederationLinkRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "remoteCanonicalKey": null,
  "remoteLabel": null,
  "relationType": null,
  "verificationStatus": null,
  "status": null,
  "metadata": null,
} satisfies UpdateEntityFederationLinkRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as UpdateEntityFederationLinkRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


