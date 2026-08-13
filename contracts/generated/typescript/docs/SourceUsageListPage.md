
# SourceUsageListPage


## Properties

Name | Type
------------ | -------------
`items` | [Array&lt;SourceUsage&gt;](SourceUsage.md)
`nextCursor` | string
`totalUsageCount` | number
`totalPageCount` | number
`totalBlockCount` | number
`totalCitationCount` | number

## Example

```typescript
import type { SourceUsageListPage } from ''

// TODO: Update the object below with actual values
const example = {
  "items": null,
  "nextCursor": null,
  "totalUsageCount": null,
  "totalPageCount": null,
  "totalBlockCount": null,
  "totalCitationCount": null,
} satisfies SourceUsageListPage

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as SourceUsageListPage
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


