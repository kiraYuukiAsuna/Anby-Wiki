
# RenderedPageSection


## Properties

Name | Type
------------ | -------------
`key` | string
`position` | number
`headingBlockId` | string
`level` | number
`title` | string
`sizeBytes` | number
`revisionId` | string
`astJson` | { [key: string]: any; }
`html` | string
`rendererVersion` | string

## Example

```typescript
import type { RenderedPageSection } from ''

// TODO: Update the object below with actual values
const example = {
  "key": null,
  "position": null,
  "headingBlockId": null,
  "level": null,
  "title": null,
  "sizeBytes": null,
  "revisionId": null,
  "astJson": null,
  "html": null,
  "rendererVersion": null,
} satisfies RenderedPageSection

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as RenderedPageSection
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


