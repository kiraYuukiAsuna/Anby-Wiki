
# PageCatalogPage


## Properties

Name | Type
------------ | -------------
`items` | [Array&lt;PageCatalogItem&gt;](PageCatalogItem.md)
`nextCursor` | string
`total` | number

## Example

```typescript
import type { PageCatalogPage } from ''

// TODO: Update the object below with actual values
const example = {
  "items": null,
  "nextCursor": null,
  "total": null,
} satisfies PageCatalogPage

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PageCatalogPage
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


