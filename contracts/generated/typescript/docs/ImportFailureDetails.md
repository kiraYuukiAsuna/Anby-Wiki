
# ImportFailureDetails


## Properties

Name | Type
------------ | -------------
`sourceVersionId` | string
`extractionId` | string
`model` | string
`promptInjectionDetected` | boolean
`qualityScore` | number
`qualityThreshold` | number
`entityCount` | number
`claimCount` | number
`extractionReused` | boolean
`providerCode` | string
`httpStatus` | number
`maxInputTokens` | number
`maxSourceBytes` | number
`maxPages` | number
`page` | number

## Example

```typescript
import type { ImportFailureDetails } from ''

// TODO: Update the object below with actual values
const example = {
  "sourceVersionId": null,
  "extractionId": null,
  "model": null,
  "promptInjectionDetected": null,
  "qualityScore": null,
  "qualityThreshold": null,
  "entityCount": null,
  "claimCount": null,
  "extractionReused": null,
  "providerCode": null,
  "httpStatus": null,
  "maxInputTokens": null,
  "maxSourceBytes": null,
  "maxPages": null,
  "page": null,
} satisfies ImportFailureDetails

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportFailureDetails
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


