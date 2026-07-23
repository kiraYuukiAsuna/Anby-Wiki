
# SubmitProposalResult


## Properties

Name | Type
------------ | -------------
`proposal` | [Proposal](Proposal.md)
`reviewTask` | [ReviewTask](ReviewTask.md)
`decision` | [RiskDecision](RiskDecision.md)

## Example

```typescript
import type { SubmitProposalResult } from ''

// TODO: Update the object below with actual values
const example = {
  "proposal": null,
  "reviewTask": null,
  "decision": null,
} satisfies SubmitProposalResult

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as SubmitProposalResult
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


