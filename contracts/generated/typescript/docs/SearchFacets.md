
# SearchFacets


## Properties

Name | Type
------------ | -------------
`namespaces` | [Array&lt;SearchFacetValue&gt;](SearchFacetValue.md)
`languages` | [Array&lt;SearchFacetValue&gt;](SearchFacetValue.md)
`entityTypes` | [Array&lt;SearchFacetValue&gt;](SearchFacetValue.md)

## Example

```typescript
import type { SearchFacets } from ''

// TODO: Update the object below with actual values
const example = {
  "namespaces": null,
  "languages": null,
  "entityTypes": null,
} satisfies SearchFacets

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as SearchFacets
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


