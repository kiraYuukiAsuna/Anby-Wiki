
# PageReference


## Properties

Name | Type
------------ | -------------
`number` | number
`citationId` | string
`occurrenceCount` | number
`firstBlockId` | string
`firstNodeId` | string
`occurrences` | [Array&lt;PageReferenceOccurrence&gt;](PageReferenceOccurrence.md)
`sourceVersionId` | string
`sourceId` | string
`sourceType` | string
`sourceTitle` | string
`author` | string
`publisher` | string
`publishedAt` | Date
`sourceMetadata` | { [key: string]: any; }
`versionHash` | string
`fetchedAt` | Date
`locator` | { [key: string]: any; }
`quotation` | string
`externalUrl` | string

## Example

```typescript
import type { PageReference } from ''

// TODO: Update the object below with actual values
const example = {
  "number": null,
  "citationId": null,
  "occurrenceCount": null,
  "firstBlockId": null,
  "firstNodeId": null,
  "occurrences": null,
  "sourceVersionId": null,
  "sourceId": null,
  "sourceType": null,
  "sourceTitle": null,
  "author": null,
  "publisher": null,
  "publishedAt": null,
  "sourceMetadata": null,
  "versionHash": null,
  "fetchedAt": null,
  "locator": null,
  "quotation": null,
  "externalUrl": null,
} satisfies PageReference

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PageReference
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


