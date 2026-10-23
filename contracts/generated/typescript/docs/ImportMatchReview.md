
# ImportMatchReview


## Properties

Name | Type
------------ | -------------
`resolutions` | [Array&lt;ImportEntityResolution&gt;](ImportEntityResolution.md)
`candidates` | [Array&lt;ImportEntityMatchReview&gt;](ImportEntityMatchReview.md)
`skippedClaims` | [Array&lt;ImportSkippedClaimReview&gt;](ImportSkippedClaimReview.md)

## Example

```typescript
import type { ImportMatchReview } from ''

// TODO: Update the object below with actual values
const example = {
  "resolutions": null,
  "candidates": null,
  "skippedClaims": null,
} satisfies ImportMatchReview

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportMatchReview
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


