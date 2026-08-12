
# RelatedPage


## Properties

Name | Type
------------ | -------------
`pageId` | string
`displayTitle` | string
`score` | number
`reasons` | [Array&lt;RelatedReason&gt;](RelatedReason.md)

## Example

```typescript
import type { RelatedPage } from ''

// TODO: Update the object below with actual values
const example = {
  "pageId": null,
  "displayTitle": null,
  "score": null,
  "reasons": null,
} satisfies RelatedPage

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as RelatedPage
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


