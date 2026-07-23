
# ClaimDetail


## Properties

Name | Type
------------ | -------------
`id` | string
`subjectEntityId` | string
`property` | [PropertySummary](PropertySummary.md)
`valueType` | string
`value` | any
`targetEntityId` | string
`qualifiers` | { [key: string]: any; }
`rank` | string
`status` | string
`verificationStatus` | string
`validFrom` | Date
`validTo` | Date
`originType` | string
`createdBy` | string
`createdAt` | Date
`supersededBy` | string
`sources` | [Array&lt;ClaimSource&gt;](ClaimSource.md)

## Example

```typescript
import type { ClaimDetail } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "subjectEntityId": null,
  "property": null,
  "valueType": null,
  "value": null,
  "targetEntityId": null,
  "qualifiers": null,
  "rank": null,
  "status": null,
  "verificationStatus": null,
  "validFrom": null,
  "validTo": null,
  "originType": null,
  "createdBy": null,
  "createdAt": null,
  "supersededBy": null,
  "sources": null,
} satisfies ClaimDetail

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ClaimDetail
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


