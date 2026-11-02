
# UpdateSystemMailRequest


## Properties

Name | Type
------------ | -------------
`_configuration` | [SystemMailConfig](SystemMailConfig.md)
`smtpPassword` | string
`password` | string
`code` | string

## Example

```typescript
import type { UpdateSystemMailRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "_configuration": null,
  "smtpPassword": null,
  "password": null,
  "code": null,
} satisfies UpdateSystemMailRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as UpdateSystemMailRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


