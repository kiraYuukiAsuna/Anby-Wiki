
# ImportRun


## Properties

Name | Type
------------ | -------------
`id` | string
`importJobId` | string
`attempt` | number
`idempotencyKey` | string
`status` | string
`error` | { [key: string]: any; }
`startedAt` | Date
`finishedAt` | Date

## Example

```typescript
import type { ImportRun } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "importJobId": null,
  "attempt": null,
  "idempotencyKey": null,
  "status": null,
  "error": null,
  "startedAt": null,
  "finishedAt": null,
} satisfies ImportRun

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportRun
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


