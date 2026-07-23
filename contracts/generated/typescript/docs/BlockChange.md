
# BlockChange

一条 Block 级变更。同一 Block 字段与位置都变时产生 changed 与 moved 两条独立条目。 parent_id 为变更后（added/changed/moved）或变更前（removed）的父 Block ID，顶层为空串。

## Properties

Name | Type
------------ | -------------
`type` | string
`blockId` | string
`parentId` | string
`path` | Array&lt;number&gt;
`beforePath` | Array&lt;number&gt;
`afterPath` | Array&lt;number&gt;
`fields` | [Array&lt;FieldChange&gt;](FieldChange.md)

## Example

```typescript
import type { BlockChange } from ''

// TODO: Update the object below with actual values
const example = {
  "type": null,
  "blockId": null,
  "parentId": null,
  "path": null,
  "beforePath": null,
  "afterPath": null,
  "fields": null,
} satisfies BlockChange

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as BlockChange
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


