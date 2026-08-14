
# AIConfig


## Properties

Name | Type
------------ | -------------
`version` | number
`enabled` | boolean
`provider` | string
`baseUrl` | string
`model` | string
`responseFormat` | string
`maxInputTokens` | number
`chunkCharacters` | number
`requestTimeoutSeconds` | number
`maxAttempts` | number
`apiKeyConfigured` | boolean
`updatedBy` | string
`updatedAt` | Date

## Example

```typescript
import type { AIConfig } from ''

// TODO: Update the object below with actual values
const example = {
  "version": null,
  "enabled": null,
  "provider": null,
  "baseUrl": null,
  "model": null,
  "responseFormat": null,
  "maxInputTokens": null,
  "chunkCharacters": null,
  "requestTimeoutSeconds": null,
  "maxAttempts": null,
  "apiKeyConfigured": null,
  "updatedBy": null,
  "updatedAt": null,
} satisfies AIConfig

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AIConfig
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


