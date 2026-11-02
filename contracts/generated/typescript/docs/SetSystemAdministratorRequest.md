
# SetSystemAdministratorRequest


## Properties

Name | Type
------------ | -------------
`actorId` | string
`grant` | boolean
`password` | string
`code` | string

## Example

```typescript
import type { SetSystemAdministratorRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "actorId": null,
  "grant": null,
  "password": null,
  "code": null,
} satisfies SetSystemAdministratorRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as SetSystemAdministratorRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


