
# MachineActor


## Properties

Name | Type
------------ | -------------
`actorId` | string
`wikiId` | string
`actorType` | string
`displayName` | string
`ownerActorId` | string
`status` | string
`createdAt` | Date

## Example

```typescript
import type { MachineActor } from ''

// TODO: Update the object below with actual values
const example = {
  "actorId": null,
  "wikiId": null,
  "actorType": null,
  "displayName": null,
  "ownerActorId": null,
  "status": null,
  "createdAt": null,
} satisfies MachineActor

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as MachineActor
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


