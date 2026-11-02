
# SystemAccounts


## Properties

Name | Type
------------ | -------------
`accounts` | [Array&lt;SystemAccount&gt;](SystemAccount.md)
`events` | [Array&lt;SecurityEvent&gt;](SecurityEvent.md)
`accountsPage` | number
`eventsPage` | number
`pageSize` | number
`accountsTotal` | number
`eventsTotal` | number
`mfaEnabled` | boolean

## Example

```typescript
import type { SystemAccounts } from ''

// TODO: Update the object below with actual values
const example = {
  "accounts": null,
  "events": null,
  "accountsPage": null,
  "eventsPage": null,
  "pageSize": null,
  "accountsTotal": null,
  "eventsTotal": null,
  "mfaEnabled": null,
} satisfies SystemAccounts

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as SystemAccounts
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


