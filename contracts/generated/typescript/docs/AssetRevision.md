
# AssetRevision


## Properties

Name | Type
------------ | -------------
`id` | string
`assetId` | string
`contentHash` | string
`mimeType` | string
`sizeBytes` | number
`width` | number
`height` | number
`createdAt` | Date

## Example

```typescript
import type { AssetRevision } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "assetId": null,
  "contentHash": null,
  "mimeType": null,
  "sizeBytes": null,
  "width": null,
  "height": null,
  "createdAt": null,
} satisfies AssetRevision

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AssetRevision
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


