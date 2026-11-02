
# AccountSecurity


## Properties

Name | Type
------------ | -------------
`email` | string
`emailVerified` | boolean
`mfaEnabled` | boolean
`recoveryCodesRemaining` | number
`systemAdmin` | boolean
`mailEnabled` | boolean
`sessions` | [Array&lt;DeviceSession&gt;](DeviceSession.md)
`events` | [Array&lt;SecurityEvent&gt;](SecurityEvent.md)

## Example

```typescript
import type { AccountSecurity } from ''

// TODO: Update the object below with actual values
const example = {
  "email": null,
  "emailVerified": null,
  "mfaEnabled": null,
  "recoveryCodesRemaining": null,
  "systemAdmin": null,
  "mailEnabled": null,
  "sessions": null,
  "events": null,
} satisfies AccountSecurity

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AccountSecurity
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


