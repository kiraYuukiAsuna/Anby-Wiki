
# PageShareLinkAcceptanceResult


## Properties

Name | Type
------------ | -------------
`wikiId` | string
`pageId` | string
`roleKey` | string
`share` | [PageShare](PageShare.md)
`changed` | boolean

## Example

```typescript
import type { PageShareLinkAcceptanceResult } from ''

// TODO: Update the object below with actual values
const example = {
  "wikiId": null,
  "pageId": null,
  "roleKey": null,
  "share": null,
  "changed": null,
} satisfies PageShareLinkAcceptanceResult

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PageShareLinkAcceptanceResult
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


