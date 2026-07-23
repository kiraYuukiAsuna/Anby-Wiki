
# RiskDecision


## Properties

Name | Type
------------ | -------------
`level` | string
`reasons` | Array&lt;string&gt;
`autoApprove` | boolean
`policy` | string

## Example

```typescript
import type { RiskDecision } from ''

// TODO: Update the object below with actual values
const example = {
  "level": null,
  "reasons": null,
  "autoApprove": null,
  "policy": null,
} satisfies RiskDecision

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as RiskDecision
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


