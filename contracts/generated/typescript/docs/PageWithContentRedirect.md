
# PageWithContentRedirect

请求命中站内重定向源时出现；响应返回的是落地页

## Properties

Name | Type
------------ | -------------
`fromPageId` | string
`fromTitle` | string

## Example

```typescript
import type { PageWithContentRedirect } from ''

// TODO: Update the object below with actual values
const example = {
  "fromPageId": null,
  "fromTitle": null,
} satisfies PageWithContentRedirect

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PageWithContentRedirect
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


