
# MergeProposalToWorkingDocumentRequest


## Properties

Name | Type
------------ | -------------
`workingDocumentId` | string
`expectedSequence` | number
`clientId` | string
`clientUpdateId` | string
`currentAst` | { [key: string]: any; }
`mergedAst` | { [key: string]: any; }
`updateBase64` | string

## Example

```typescript
import type { MergeProposalToWorkingDocumentRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "workingDocumentId": null,
  "expectedSequence": null,
  "clientId": null,
  "clientUpdateId": null,
  "currentAst": null,
  "mergedAst": null,
  "updateBase64": null,
} satisfies MergeProposalToWorkingDocumentRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as MergeProposalToWorkingDocumentRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


