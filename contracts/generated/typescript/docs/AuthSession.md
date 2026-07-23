
# AuthSession


## Properties

Name | Type
------------ | -------------
`actorId` | string
`actorType` | string
`displayName` | string
`method` | string

## Example

```typescript
import type { AuthSession } from ''

// TODO: Update the object below with actual values
const example = {
  "actorId": null,
  "actorType": null,
  "displayName": null,
  "method": null,
} satisfies AuthSession

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AuthSession
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


