
# DocumentDiff

两版结构 Diff（按 Block ID 对齐 base=from 与 current=to）。 changes 按 current 前序排列，removed 按 base 前序追加在后，确定性输出； from == to 时为空数组。

## Properties

Name | Type
------------ | -------------
`changes` | [Array&lt;BlockChange&gt;](BlockChange.md)

## Example

```typescript
import type { DocumentDiff } from ''

// TODO: Update the object below with actual values
const example = {
  "changes": null,
} satisfies DocumentDiff

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as DocumentDiff
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


