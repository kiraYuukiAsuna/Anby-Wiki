
# CreateFederatedWikiRequest


## Properties

Name | Type
------------ | -------------
`wikiKey` | string
`displayName` | string
`baseUrl` | string
`entityUrlTemplate` | string
`trustLevel` | string
`status` | string
`metadata` | { [key: string]: any; }

## Example

```typescript
import type { CreateFederatedWikiRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "wikiKey": null,
  "displayName": null,
  "baseUrl": null,
  "entityUrlTemplate": null,
  "trustLevel": null,
  "status": null,
  "metadata": null,
} satisfies CreateFederatedWikiRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CreateFederatedWikiRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


