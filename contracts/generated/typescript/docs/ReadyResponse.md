
# ReadyResponse


## Properties

Name | Type
------------ | -------------
`status` | string
`checks` | { [key: string]: string; }

## Example

```typescript
import type { ReadyResponse } from ''

// TODO: Update the object below with actual values
const example = {
  "status": null,
  "checks": {postgres=ok, redis=not_configured},
} satisfies ReadyResponse

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ReadyResponse
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


