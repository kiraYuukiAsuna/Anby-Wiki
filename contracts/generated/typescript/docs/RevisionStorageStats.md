
# RevisionStorageStats


## Properties

Name | Type
------------ | -------------
`archiveAvailable` | boolean
`retentionSeconds` | number
`defaultBatchSize` | number
`hotSnapshots` | number
`coldSnapshots` | number
`hotBytes` | number
`coldBytes` | number
`eligibleSnapshots` | number
`oldestHotCreatedAt` | Date

## Example

```typescript
import type { RevisionStorageStats } from ''

// TODO: Update the object below with actual values
const example = {
  "archiveAvailable": null,
  "retentionSeconds": null,
  "defaultBatchSize": null,
  "hotSnapshots": null,
  "coldSnapshots": null,
  "hotBytes": null,
  "coldBytes": null,
  "eligibleSnapshots": null,
  "oldestHotCreatedAt": null,
} satisfies RevisionStorageStats

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as RevisionStorageStats
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


