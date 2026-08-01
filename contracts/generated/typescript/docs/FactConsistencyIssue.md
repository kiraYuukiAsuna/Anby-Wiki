
# FactConsistencyIssue


## Properties

Name | Type
------------ | -------------
`id` | string
`wikiId` | string
`subjectEntityId` | string
`subjectLabel` | string
`propertyId` | string
`propertyKey` | string
`propertyName` | string
`issueKey` | string
`issueType` | string
`severity` | string
`claimIds` | Array&lt;string&gt;
`details` | { [key: string]: any; }
`status` | string
`detectedAt` | Date
`lastCheckedAt` | Date
`resolvedAt` | Date

## Example

```typescript
import type { FactConsistencyIssue } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "wikiId": null,
  "subjectEntityId": null,
  "subjectLabel": null,
  "propertyId": null,
  "propertyKey": null,
  "propertyName": null,
  "issueKey": null,
  "issueType": null,
  "severity": null,
  "claimIds": null,
  "details": null,
  "status": null,
  "detectedAt": null,
  "lastCheckedAt": null,
  "resolvedAt": null,
} satisfies FactConsistencyIssue

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as FactConsistencyIssue
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


