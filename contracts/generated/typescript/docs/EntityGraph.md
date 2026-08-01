
# EntityGraph


## Properties

Name | Type
------------ | -------------
`rootId` | string
`direction` | string
`propertyKey` | string
`requestedDepth` | number
`reachedDepth` | number
`nodes` | [Array&lt;EntityGraphNode&gt;](EntityGraphNode.md)
`edges` | [Array&lt;EntityGraphEdge&gt;](EntityGraphEdge.md)
`truncated` | boolean
`projectionUpdatedAt` | Date

## Example

```typescript
import type { EntityGraph } from ''

// TODO: Update the object below with actual values
const example = {
  "rootId": null,
  "direction": null,
  "propertyKey": null,
  "requestedDepth": null,
  "reachedDepth": null,
  "nodes": null,
  "edges": null,
  "truncated": null,
  "projectionUpdatedAt": null,
} satisfies EntityGraph

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as EntityGraph
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


