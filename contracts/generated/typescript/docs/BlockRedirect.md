
# BlockRedirect


## Properties

Name | Type
------------ | -------------
`sourcePageId` | string
`sourceBlockId` | string
`targetPageId` | string
`targetBlockId` | string
`targetPageTitle` | string
`targetCurrentSlug` | string
`createdBy` | string
`createdAt` | Date

## Example

```typescript
import type { BlockRedirect } from ''

// TODO: Update the object below with actual values
const example = {
  "sourcePageId": null,
  "sourceBlockId": null,
  "targetPageId": null,
  "targetBlockId": null,
  "targetPageTitle": null,
  "targetCurrentSlug": null,
  "createdBy": null,
  "createdAt": null,
} satisfies BlockRedirect

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as BlockRedirect
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


