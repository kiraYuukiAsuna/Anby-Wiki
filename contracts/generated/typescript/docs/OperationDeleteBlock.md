
# OperationDeleteBlock


## Properties

Name | Type
------------ | -------------
`schemaVersion` | number
`operationType` | string
`base` | [OperationBase](OperationBase.md)
`target` | [OperationTarget](OperationTarget.md)
`expectedHash` | string
`evidence` | [Array&lt;OperationEvidence&gt;](OperationEvidence.md)
`risk` | [OperationRisk](OperationRisk.md)
`payload` | { [key: string]: any; }

## Example

```typescript
import type { OperationDeleteBlock } from ''

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
} satisfies OperationDeleteBlock

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as OperationDeleteBlock
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


