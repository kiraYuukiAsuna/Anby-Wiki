
# CreatePageProtectionRequest


## Properties

Name | Type
------------ | -------------
`pageId` | string
`namespaceKey` | string
`normalizedTitle` | string
`actionType` | string
`requiredRoleKey` | string
`expiresAt` | Date

## Example

```typescript
import type { CreatePageProtectionRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "pageId": null,
  "namespaceKey": null,
  "normalizedTitle": null,
  "actionType": null,
  "requiredRoleKey": null,
  "expiresAt": null,
} satisfies CreatePageProtectionRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as CreatePageProtectionRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


