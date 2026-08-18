
# CLIAuthCode


## Properties

Name | Type
------------ | -------------
`code` | string
`name` | string
`expiresAt` | Date
`tokenExpiresAt` | Date

## Example

```typescript
import type { CLIAuthCode } from ''

// TODO: Update the object below with actual values
const example = {
  "code": null,
  "name": null,
  "expiresAt": null,
  "tokenExpiresAt": null,
} satisfies CLIAuthCode

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CLIAuthCode
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


