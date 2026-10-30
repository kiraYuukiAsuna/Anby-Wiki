
# CLIPendingAuthorizationCode


## Properties

Name | Type
------------ | -------------
`actorId` | string
`wikiId` | string
`actorType` | string
`displayName` | string
`id` | string
`name` | string
`status` | string
`expiresAt` | Date
`tokenExpiresAt` | Date
`createdAt` | Date

## Example

```typescript
import type { CLIPendingAuthorizationCode } from ''

// TODO: Update the object below with actual values
const example = {
  "actorId": null,
  "wikiId": null,
  "actorType": null,
  "displayName": null,
  "id": null,
  "name": null,
  "status": null,
  "expiresAt": null,
  "tokenExpiresAt": null,
  "createdAt": null,
} satisfies CLIPendingAuthorizationCode

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CLIPendingAuthorizationCode
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


