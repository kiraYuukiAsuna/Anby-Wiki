
# OutlineItem

文档目录条目（heading 层级树的一个节点）

## Properties

Name | Type
------------ | -------------
`headingBlockId` | string
`parentHeadingBlockId` | string
`level` | number
`title` | string
`slug` | string
`positionKey` | string

## Example

```typescript
import type { OutlineItem } from ''

// TODO: Update the object below with actual values
const example = {
  "headingBlockId": null,
  "parentHeadingBlockId": null,
  "level": null,
  "title": null,
  "slug": story-2,
  "positionKey": 1.2.3,
} satisfies OutlineItem

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as OutlineItem
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


