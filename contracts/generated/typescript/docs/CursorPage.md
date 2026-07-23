
# CursorPage

统一游标分页信封；列表端点以内联 allOf 方式复用

## Properties

Name | Type
------------ | -------------
`items` | Array&lt;any&gt;
`nextCursor` | string

## Example

```typescript
import type { CursorPage } from ''

// TODO: Update the object below with actual values
const example = {
  "items": null,
  "nextCursor": null,
} satisfies CursorPage

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CursorPage
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


