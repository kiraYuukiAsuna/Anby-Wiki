
# ReviewTask


## Properties

Name | Type
------------ | -------------
`id` | string
`proposalId` | string
`status` | string
`reviewerId` | string
`decisionReason` | string
`createdAt` | Date
`reviewedAt` | Date

## Example

```typescript
import type { ReviewTask } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "proposalId": null,
  "status": null,
  "reviewerId": null,
  "decisionReason": null,
  "createdAt": null,
  "reviewedAt": null,
} satisfies ReviewTask

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ReviewTask
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


