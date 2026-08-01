
# SnapshotArchiveResult


## Properties

Name | Type
------------ | -------------
`examined` | number
`archived` | number
`skipped` | number
`archivedBytes` | number

## Example

```typescript
import type { SnapshotArchiveResult } from ''

// TODO: Update the object below with actual values
const example = {
  "examined": null,
  "archived": null,
  "skipped": null,
  "archivedBytes": null,
} satisfies SnapshotArchiveResult

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as SnapshotArchiveResult
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


