
# CitationDetail


## Properties

Name | Type
------------ | -------------
`id` | string
`sourceVersionId` | string
`sourceChunkId` | string
`locator` | { [key: string]: any; }
`quotation` | string
`quotationHash` | string
`createdBy` | string
`createdAt` | Date
`source` | [SourceSummary](SourceSummary.md)
`sourceVersion` | [SourceVersionSummary](SourceVersionSummary.md)
`sourceChunk` | [SourceChunk](SourceChunk.md)
`externalResource` | [ExternalResourceSummary](ExternalResourceSummary.md)

## Example

```typescript
import type { CitationDetail } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "sourceVersionId": null,
  "sourceChunkId": null,
  "locator": null,
  "quotation": null,
  "quotationHash": null,
  "createdBy": null,
  "createdAt": null,
  "source": null,
  "sourceVersion": null,
  "sourceChunk": null,
  "externalResource": null,
} satisfies CitationDetail

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CitationDetail
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


