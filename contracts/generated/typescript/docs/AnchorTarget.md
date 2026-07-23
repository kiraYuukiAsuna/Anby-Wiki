
# AnchorTarget

当前可导航章节目标；Block ID 是权威身份，slug 只用于展示地址

## Properties

Name | Type
------------ | -------------
`pageId` | string
`blockId` | string
`currentSlug` | string
`viaAlias` | boolean
`viaRedirect` | boolean

## Example

```typescript
import type { AnchorTarget } from ''

// TODO: Update the object below with actual values
const example = {
  "pageId": null,
  "blockId": null,
  "currentSlug": null,
  "viaAlias": null,
  "viaRedirect": null,
} satisfies AnchorTarget

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as AnchorTarget
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


