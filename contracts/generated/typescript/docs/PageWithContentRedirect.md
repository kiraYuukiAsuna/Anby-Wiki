
# PageWithContentRedirect

请求命中重定向源时出现。resolved=true 时 page/content 是本地落地页； unresolved 或 interwiki 终点返回 resolved=false、content=null，并由 target 给出可治理的目标。

## Properties

Name | Type
------------ | -------------
`fromPageId` | string
`fromTitle` | string
`target` | [PageRedirectTarget](PageRedirectTarget.md)
`resolved` | boolean
`hops` | number

## Example

```typescript
import type { PageWithContentRedirect } from ''

// TODO: Update the object below with actual values
const example = {
  "fromPageId": null,
  "fromTitle": null,
  "target": null,
  "resolved": null,
  "hops": null,
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


