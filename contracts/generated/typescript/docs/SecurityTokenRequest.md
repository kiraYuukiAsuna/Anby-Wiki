
# SecurityTokenRequest


## Properties

Name | Type
------------ | -------------
`token` | string
`purpose` | string
`password` | string
`code` | string

## Example

```typescript
import type { SecurityTokenRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "token": null,
  "purpose": null,
  "password": null,
  "code": null,
} satisfies SecurityTokenRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as SecurityTokenRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


