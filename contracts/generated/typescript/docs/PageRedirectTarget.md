
# PageRedirectTarget

判别式重定向目标。page 需要 target_page_id；page_section 还需要 anchor_block_id；unresolved 需要 namespace 与 target_title； interwiki 需要 target_title 与 external_url。

## Properties

Name | Type
------------ | -------------
`kind` | string
`targetPageId` | string
`targetPageTitle` | string
`namespaceId` | string
`namespace` | string
`targetTitle` | string
`anchorBlockId` | string
`externalUrl` | string

## Example

```typescript
import type { PageRedirectTarget } from ''

// TODO: Update the object below with actual values
const example = {
  "kind": null,
  "targetPageId": null,
  "targetPageTitle": null,
  "namespaceId": null,
  "namespace": null,
  "targetTitle": null,
  "anchorBlockId": null,
  "externalUrl": null,
} satisfies PageRedirectTarget

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PageRedirectTarget
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


