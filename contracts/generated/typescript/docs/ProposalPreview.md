
# ProposalPreview


## Properties

Name | Type
------------ | -------------
`proposalId` | string
`targetType` | string
`riskLevel` | string
`stale` | boolean
`base` | [PreviewDocument](PreviewDocument.md)
`current` | [PreviewDocument](PreviewDocument.md)
`proposed` | [PreviewDocument](PreviewDocument.md)
`baseToCurrent` | [DocumentDiff](DocumentDiff.md)
`baseToProposed` | [DocumentDiff](DocumentDiff.md)
`evidence` | [Array&lt;OperationEvidence&gt;](OperationEvidence.md)
`impact` | [PreviewImpact](PreviewImpact.md)

## Example

```typescript
import type { ProposalPreview } from ''

// TODO: Update the object below with actual values
const example = {
  "proposalId": null,
  "targetType": null,
  "riskLevel": null,
  "stale": null,
  "base": null,
  "current": null,
  "proposed": null,
  "baseToCurrent": null,
  "baseToProposed": null,
  "evidence": null,
  "impact": null,
} satisfies ProposalPreview

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ProposalPreview
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


