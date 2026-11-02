
# SystemMailConfig


## Properties

Name | Type
------------ | -------------
`enabled` | boolean
`host` | string
`port` | number
`tlsMode` | string
`username` | string
`fromEmail` | string
`fromName` | string
`publicUrl` | string
`passwordConfigured` | boolean
`updatedAt` | Date

## Example

```typescript
import type { SystemMailConfig } from ''

// TODO: Update the object below with actual values
const example = {
  "enabled": null,
  "host": null,
  "port": null,
  "tlsMode": null,
  "username": null,
  "fromEmail": null,
  "fromName": null,
  "publicUrl": null,
  "passwordConfigured": null,
  "updatedAt": null,
} satisfies SystemMailConfig

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as SystemMailConfig
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


