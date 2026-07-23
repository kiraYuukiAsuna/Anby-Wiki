
# ImportStageRun


## Properties

Name | Type
------------ | -------------
`id` | string
`importRunId` | string
`stage` | string
`status` | string
`inputHash` | string
`outputHash` | string
`error` | { [key: string]: any; }
`startedAt` | Date
`finishedAt` | Date

## Example

```typescript
import type { ImportStageRun } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "importRunId": null,
  "stage": null,
  "status": null,
  "inputHash": null,
  "outputHash": null,
  "error": null,
  "startedAt": null,
  "finishedAt": null,
} satisfies ImportStageRun

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportStageRun
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


