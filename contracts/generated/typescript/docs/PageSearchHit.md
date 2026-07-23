
# PageSearchHit

页面搜索结果条目

## Properties

Name | Type
------------ | -------------
`id` | string
`displayTitle` | string
`namespace` | string
`matchedOn` | string
`highlight` | string
`score` | number

## Example

```typescript
import type { PageSearchHit } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "displayTitle": Anby Demara,
  "namespace": main,
  "matchedOn": null,
  "highlight": A quiet [[swordswoman]] from the Cunning Hares.,
  "score": null,
} satisfies PageSearchHit

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PageSearchHit
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


