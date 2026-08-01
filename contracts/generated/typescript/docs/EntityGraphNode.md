
# EntityGraphNode


## Properties

Name | Type
------------ | -------------
`id` | string
`canonicalKey` | string
`status` | string
`entityTypeKey` | string
`entityTypeName` | string
`label` | string
`language` | string
`description` | string
`depth` | number

## Example

```typescript
import type { EntityGraphNode } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "canonicalKey": null,
  "status": null,
  "entityTypeKey": null,
  "entityTypeName": null,
  "label": null,
  "language": null,
  "description": null,
  "depth": null,
} satisfies EntityGraphNode

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as EntityGraphNode
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


