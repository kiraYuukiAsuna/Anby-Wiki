
# UpdateAIConfigRequest


## Properties

Name | Type
------------ | -------------
`enabled` | boolean
`provider` | string
`baseUrl` | string
`model` | string
`responseFormat` | string
`maxInputTokens` | number
`requestTimeoutSeconds` | number
`maxAttempts` | number
`apiKey` | string

## Example

```typescript
import type { UpdateAIConfigRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "enabled": null,
  "provider": null,
  "baseUrl": null,
  "model": null,
  "responseFormat": null,
  "maxInputTokens": null,
  "requestTimeoutSeconds": null,
  "maxAttempts": null,
  "apiKey": null,
} satisfies UpdateAIConfigRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as UpdateAIConfigRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


