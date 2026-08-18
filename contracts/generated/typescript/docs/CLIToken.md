
# CLIToken


## Properties

Name | Type
------------ | -------------
`id` | string
`name` | string
`tokenPrefix` | string
`status` | string
`expiresAt` | Date
`lastUsedAt` | Date
`revokedAt` | Date
`createdAt` | Date

## Example

```typescript
import type { CLIToken } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "name": null,
  "tokenPrefix": null,
  "status": null,
  "expiresAt": null,
  "lastUsedAt": null,
  "revokedAt": null,
  "createdAt": null,
} satisfies CLIToken

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CLIToken
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


