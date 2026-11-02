
# AccountSecurityRequest


## Properties

Name | Type
------------ | -------------
`action` | string
`password` | string
`code` | string
`email` | string
`newPassword` | string
`sessionId` | string

## Example

```typescript
import type { AccountSecurityRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "action": null,
  "password": null,
  "code": null,
  "email": null,
  "newPassword": null,
  "sessionId": null,
} satisfies AccountSecurityRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AccountSecurityRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


