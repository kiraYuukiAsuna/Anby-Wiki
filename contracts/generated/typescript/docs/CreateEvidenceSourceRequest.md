
# CreateEvidenceSourceRequest


## Properties

Name | Type
------------ | -------------
`sourceType` | string
`externalResourceId` | string
`url` | string
`assetId` | string
`title` | string
`author` | string
`publisher` | string
`publishedAt` | Date
`metadata` | { [key: string]: any; }

## Example

```typescript
import type { CreateEvidenceSourceRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "sourceType": null,
  "externalResourceId": null,
  "url": null,
  "assetId": null,
  "title": null,
  "author": null,
  "publisher": null,
  "publishedAt": null,
  "metadata": null,
} satisfies CreateEvidenceSourceRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CreateEvidenceSourceRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


