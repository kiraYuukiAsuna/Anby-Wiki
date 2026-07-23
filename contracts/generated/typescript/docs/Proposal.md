
# Proposal


## Properties

Name | Type
------------ | -------------
`id` | string
`importJobId` | string
`targetType` | string
`targetId` | string
`baseRevisionId` | string
`baseStateVersion` | number
`status` | string
`riskLevel` | string
`riskReasons` | Array&lt;string&gt;
`policyDecision` | { [key: string]: any; }
`createdBy` | string
`idempotencyKey` | string
`createdAt` | Date
`updatedAt` | Date
`operations` | [Array&lt;ProposalOperationRecord&gt;](ProposalOperationRecord.md)
`conflicts` | [Array&lt;MergeConflict&gt;](MergeConflict.md)

## Example

```typescript
import type { Proposal } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "importJobId": null,
  "targetType": null,
  "targetId": null,
  "baseRevisionId": null,
  "baseStateVersion": null,
  "status": null,
  "riskLevel": null,
  "riskReasons": null,
  "policyDecision": null,
  "createdBy": null,
  "idempotencyKey": null,
  "createdAt": null,
  "updatedAt": null,
  "operations": null,
  "conflicts": null,
} satisfies Proposal

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as Proposal
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


