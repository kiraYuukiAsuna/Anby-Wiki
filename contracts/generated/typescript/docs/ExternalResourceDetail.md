
# ExternalResourceDetail


## Properties

Name | Type
------------ | -------------
`id` | string
`originalUrl` | string
`normalizedUrl` | string
`canonicalUrl` | string
`domain` | string
`httpStatus` | number
`status` | string
`lastCheckedAt` | Date
`lastSuccessAt` | Date
`consecutiveFailures` | number

## Example

```typescript
import type { ExternalResourceDetail } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "originalUrl": null,
  "normalizedUrl": null,
  "canonicalUrl": null,
  "domain": null,
  "httpStatus": null,
  "status": null,
  "lastCheckedAt": null,
  "lastSuccessAt": null,
  "consecutiveFailures": null,
} satisfies ExternalResourceDetail

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ExternalResourceDetail
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


