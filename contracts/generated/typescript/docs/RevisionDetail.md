
# RevisionDetail

单版详情：Revision 元信息 + canonical AST（不含 html）

## Properties

Name | Type
------------ | -------------
`revision` | [Revision](Revision.md)
`astJson` | { [key: string]: any; }

## Example

```typescript
import type { RevisionDetail } from ''

// TODO: Update the object below with actual values
const example = {
  "revision": null,
  "astJson": null,
} satisfies RevisionDetail

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as RevisionDetail
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


