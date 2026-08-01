
# CreateCitationRequest


## Properties

Name | Type
------------ | -------------
`sourceVersionId` | string
`sourceChunkId` | string
`locator` | [EvidenceLocator](EvidenceLocator.md)
`quotation` | string

## Example

```typescript
import type { CreateCitationRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "sourceVersionId": null,
  "sourceChunkId": null,
  "locator": null,
  "quotation": null,
} satisfies CreateCitationRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CreateCitationRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


