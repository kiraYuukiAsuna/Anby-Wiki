
# WikiComponentVersion


## Properties

Name | Type
------------ | -------------
`componentId` | string
`version` | number
`propsSchema` | { [key: string]: any; }
`rendererRef` | string
`status` | string
`createdBy` | string
`createdAt` | Date
`publishedAt` | Date

## Example

```typescript
import type { WikiComponentVersion } from ''

// TODO: Update the object below with actual values
const example = {
  "componentId": null,
  "version": null,
  "propsSchema": null,
  "rendererRef": null,
  "status": null,
  "createdBy": null,
  "createdAt": null,
  "publishedAt": null,
} satisfies WikiComponentVersion

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as WikiComponentVersion
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


