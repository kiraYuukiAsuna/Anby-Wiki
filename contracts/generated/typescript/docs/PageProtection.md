
# PageProtection


## Properties

Name | Type
------------ | -------------
`id` | string
`pageId` | string
`pageTitle` | string
`namespaceId` | string
`namespaceKey` | string
`normalizedTitle` | string
`actionType` | string
`requiredRoleId` | string
`requiredRoleKey` | string
`requiredRoleName` | string
`expiresAt` | Date
`createdBy` | string
`createdAt` | Date
`revokedAt` | Date
`revokedBy` | string

## Example

```typescript
import type { PageProtection } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "pageId": null,
  "pageTitle": null,
  "namespaceId": null,
  "namespaceKey": null,
  "normalizedTitle": null,
  "actionType": null,
  "requiredRoleId": null,
  "requiredRoleKey": null,
  "requiredRoleName": null,
  "expiresAt": null,
  "createdBy": null,
  "createdAt": null,
  "revokedAt": null,
  "revokedBy": null,
} satisfies PageProtection

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PageProtection
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


