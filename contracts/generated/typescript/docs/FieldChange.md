
# FieldChange

单字段 before/after 摘要（值为 canonical JSON 文本，单侧不存在为 \"(absent)\"）

## Properties

Name | Type
------------ | -------------
`field` | string
`before` | string
`after` | string

## Example

```typescript
import type { FieldChange } from ''

// TODO: Update the object below with actual values
const example = {
  "field": null,
  "before": null,
  "after": null,
} satisfies FieldChange

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as FieldChange
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


