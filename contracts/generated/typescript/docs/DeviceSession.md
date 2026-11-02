
# DeviceSession


## Properties

Name | Type
------------ | -------------
`id` | string
`userAgent` | string
`createdAt` | Date
`lastSeenAt` | Date
`expiresAt` | Date
`current` | boolean

## Example

```typescript
import type { DeviceSession } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "userAgent": null,
  "createdAt": null,
  "lastSeenAt": null,
  "expiresAt": null,
  "current": null,
} satisfies DeviceSession

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as DeviceSession
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


