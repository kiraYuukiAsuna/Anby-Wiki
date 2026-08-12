
# SourceImportConfig


## Properties

Name | Type
------------ | -------------
`source` | [SourceImportSource](SourceImportSource.md)
`title` | string
`instructions` | string
`routeMode` | string
`pageId` | string
`sourceId` | string
`qualityThreshold` | number

## Example

```typescript
import type { SourceImportConfig } from ''

// TODO: Update the object below with actual values
const example = {
  "source": null,
  "title": null,
  "instructions": null,
  "routeMode": null,
  "pageId": null,
  "sourceId": null,
  "qualityThreshold": null,
} satisfies SourceImportConfig

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as SourceImportConfig
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


