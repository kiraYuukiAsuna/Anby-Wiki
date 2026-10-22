
# AccountProfile


## Properties

Name | Type
------------ | -------------
`actorId` | string
`wikiId` | string
`username` | string
`email` | string
`displayName` | string
`wikiName` | string
`createdAt` | Date
`updatedAt` | Date

## Example

```typescript
import type { AccountProfile } from ''

// TODO: Update the object below with actual values
const example = {
  "actorId": null,
  "wikiId": null,
  "username": null,
  "email": null,
  "displayName": null,
  "wikiName": null,
  "createdAt": null,
  "updatedAt": null,
} satisfies AccountProfile

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AccountProfile
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


