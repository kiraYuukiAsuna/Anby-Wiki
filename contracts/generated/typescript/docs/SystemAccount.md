
# SystemAccount


## Properties

Name | Type
------------ | -------------
`actorId` | string
`username` | string
`email` | string
`systemAdmin` | boolean
`emailVerified` | boolean
`mfaEnabled` | boolean
`status` | string

## Example

```typescript
import type { SystemAccount } from ''

// TODO: Update the object below with actual values
const example = {
  "actorId": null,
  "username": null,
  "email": null,
  "systemAdmin": null,
  "emailVerified": null,
  "mfaEnabled": null,
  "status": null,
} satisfies SystemAccount

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as SystemAccount
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


