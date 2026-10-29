
# ImportFailure

导入失败诊断 v1。旧记录允许仅含 stage/code；人工实体确认仍携带 match_review。仅返回安全诊断，不返回原文、Prompt、密钥或供应商原始响应。

## Properties

Name | Type
------------ | -------------
`schemaVersion` | number
`stage` | string
`code` | string
`message` | string
`suggestion` | string
`retryable` | boolean
`details` | [ImportFailureDetails](ImportFailureDetails.md)
`matchReview` | [ImportMatchReview](ImportMatchReview.md)

## Example

```typescript
import type { ImportFailure } from ''

// TODO: Update the object below with actual values
const example = {
  "schemaVersion": null,
  "stage": null,
  "code": null,
  "message": null,
  "suggestion": null,
  "retryable": null,
  "details": null,
  "matchReview": null,
} satisfies ImportFailure

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportFailure
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


