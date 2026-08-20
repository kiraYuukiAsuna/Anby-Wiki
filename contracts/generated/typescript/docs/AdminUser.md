
# AdminUser


## Properties

Name | Type
------------ | -------------
`actorId` | string
`username` | string
`email` | string
`displayName` | string
`status` | string
`roles` | [Array&lt;RoleSummary&gt;](RoleSummary.md)
`createdAt` | Date

## Example

```typescript
import type { AdminUser } from ''

// TODO: Update the object below with actual values
const example = {
  "actorId": null,
  "username": null,
  "email": null,
  "displayName": null,
  "status": null,
  "roles": null,
  "createdAt": null,
} satisfies AdminUser

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AdminUser
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


