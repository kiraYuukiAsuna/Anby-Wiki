
# EvidenceSourceChunk


## Properties

Name | Type
------------ | -------------
`id` | string
`ordinal` | number
`locator` | [EvidenceLocator](EvidenceLocator.md)
`textContent` | string
`textHash` | string

## Example

```typescript
import type { EvidenceSourceChunk } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "ordinal": null,
  "locator": null,
  "textContent": null,
  "textHash": null,
} satisfies EvidenceSourceChunk

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as EvidenceSourceChunk
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


