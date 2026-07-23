
# MergeConflict


## Properties

Name | Type
------------ | -------------
`id` | string
`proposalId` | string
`pageId` | string
`conflictType` | string
`targetBlockId` | string
`targetClaimId` | string
`baseRevisionId` | string
`currentRevisionId` | string
`baseValue` | { [key: string]: any; }
`currentValue` | { [key: string]: any; }
`proposedValue` | { [key: string]: any; }
`status` | string
`resolvedBy` | string
`resolution` | { [key: string]: any; }
`resolvedAt` | Date
`createdAt` | Date

## Example

```typescript
import type { MergeConflict } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "proposalId": null,
  "pageId": null,
  "conflictType": null,
  "targetBlockId": null,
  "targetClaimId": null,
  "baseRevisionId": null,
  "currentRevisionId": null,
  "baseValue": null,
  "currentValue": null,
  "proposedValue": null,
  "status": null,
  "resolvedBy": null,
  "resolution": null,
  "resolvedAt": null,
  "createdAt": null,
} satisfies MergeConflict

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as MergeConflict
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


