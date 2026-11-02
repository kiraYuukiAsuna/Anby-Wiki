
# AccountSecurityResult


## Properties

Name | Type
------------ | -------------
`signedOut` | boolean
`setupSecret` | string
`setupUri` | string
`recoveryCodes` | Array&lt;string&gt;

## Example

```typescript
import type { AccountSecurityResult } from ''

// TODO: Update the object below with actual values
const example = {
  "signedOut": null,
  "setupSecret": null,
  "setupUri": null,
  "recoveryCodes": null,
} satisfies AccountSecurityResult

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AccountSecurityResult
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


