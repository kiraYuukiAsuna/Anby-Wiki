
# EvidenceSourceVersion


## Properties

Name | Type
------------ | -------------
`id` | string
`sourceId` | string
`versionHash` | string
`rawAssetId` | string
`extractedAssetId` | string
`fetchedAt` | Date
`createdAt` | Date

## Example

```typescript
import type { EvidenceSourceVersion } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "sourceId": null,
  "versionHash": null,
  "rawAssetId": null,
  "extractedAssetId": null,
  "fetchedAt": null,
  "createdAt": null,
} satisfies EvidenceSourceVersion

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as EvidenceSourceVersion
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


