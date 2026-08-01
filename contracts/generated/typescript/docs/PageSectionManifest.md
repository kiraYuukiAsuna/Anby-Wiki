
# PageSectionManifest


## Properties

Name | Type
------------ | -------------
`ready` | boolean
`revisionId` | string
`rendererVersion` | string
`citationOrder` | Array&lt;string&gt;
`items` | [Array&lt;PageSectionSummary&gt;](PageSectionSummary.md)

## Example

```typescript
import type { PageSectionManifest } from ''

// TODO: Update the object below with actual values
const example = {
  "ready": null,
  "revisionId": null,
  "rendererVersion": null,
  "citationOrder": null,
  "items": null,
} satisfies PageSectionManifest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PageSectionManifest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


