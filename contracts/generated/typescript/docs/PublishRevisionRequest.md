
# PublishRevisionRequest


## Properties

Name | Type
------------ | -------------
`expectedRevisionId` | string
`workingDocumentId` | string
`ast` | { [key: string]: any; }
`summary` | string
`isMinor` | boolean

## Example

```typescript
import type { PublishRevisionRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "expectedRevisionId": null,
  "workingDocumentId": null,
  "ast": null,
  "summary": 初版,
  "isMinor": null,
} satisfies PublishRevisionRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PublishRevisionRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


