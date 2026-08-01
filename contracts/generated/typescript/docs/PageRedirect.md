
# PageRedirect


## Properties

Name | Type
------------ | -------------
`sourcePageId` | string
`target` | [PageRedirectTarget](PageRedirectTarget.md)
`createdBy` | string
`updatedBy` | string
`createdAt` | Date
`updatedAt` | Date

## Example

```typescript
import type { PageRedirect } from ''

// TODO: Update the object below with actual values
const example = {
  "sourcePageId": null,
  "target": null,
  "createdBy": null,
  "updatedBy": null,
  "createdAt": null,
  "updatedAt": null,
} satisfies PageRedirect

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PageRedirect
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


