
# OperationSetPageEntityBinding


## Properties

Name | Type
------------ | -------------
`schemaVersion` | number
`operationType` | string
`base` | [OperationBase](OperationBase.md)
`target` | [OperationSetPageEntityBindingAllOfTarget](OperationSetPageEntityBindingAllOfTarget.md)
`expectedHash` | string
`evidence` | [Array&lt;OperationEvidence&gt;](OperationEvidence.md)
`risk` | [OperationRisk](OperationRisk.md)
`payload` | [OperationSetPageEntityBindingAllOfPayload](OperationSetPageEntityBindingAllOfPayload.md)

## Example

```typescript
import type { OperationSetPageEntityBinding } from ''

// TODO: Update the object below with actual values
const example = {
  "schemaVersion": null,
  "operationType": null,
  "base": null,
  "target": null,
  "expectedHash": null,
  "evidence": null,
  "risk": null,
  "payload": null,
} satisfies OperationSetPageEntityBinding

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as OperationSetPageEntityBinding
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


