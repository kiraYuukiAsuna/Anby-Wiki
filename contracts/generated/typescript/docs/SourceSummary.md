
# SourceSummary


## Properties

Name | Type
------------ | -------------
`id` | string
`sourceType` | string
`title` | string
`author` | string
`publisher` | string
`publishedAt` | Date
`metadata` | { [key: string]: any; }

## Example

```typescript
import type { SourceSummary } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "sourceType": null,
  "title": null,
  "author": null,
  "publisher": null,
  "publishedAt": null,
  "metadata": null,
} satisfies SourceSummary

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as SourceSummary
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


