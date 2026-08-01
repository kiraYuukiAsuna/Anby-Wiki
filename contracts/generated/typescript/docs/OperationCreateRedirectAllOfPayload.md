
# OperationCreateRedirectAllOfPayload


## Properties

Name | Type
------------ | -------------
`targetKind` | string
`targetPageId` | string
`targetNamespaceId` | string
`targetTitle` | string
`targetAnchorBlockId` | string
`targetInterwiki` | string

## Example

```typescript
import type { OperationCreateRedirectAllOfPayload } from ''

// TODO: Update the object below with actual values
const example = {
  "targetKind": null,
  "targetPageId": null,
  "targetNamespaceId": null,
  "targetTitle": null,
  "targetAnchorBlockId": null,
  "targetInterwiki": null,
} satisfies OperationCreateRedirectAllOfPayload

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as OperationCreateRedirectAllOfPayload
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


