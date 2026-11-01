
# ImportSafetyReview

受保护的任务原文预览，最多 8 个片段、每个最多 800 字；不是全文。未定位到可疑词时返回前 3 个片段。复核前应阅读完整来源。

## Properties

Name | Type
------------ | -------------
`schemaVersion` | number
`sourceId` | string
`sourceVersionId` | string
`extractionId` | string
`reviewable` | boolean
`canReview` | boolean
`directControl` | boolean
`samples` | [Array&lt;ImportSafetySourceSample&gt;](ImportSafetySourceSample.md)
`decision` | [ImportSafetyDecision](ImportSafetyDecision.md)

## Example

```typescript
import type { ImportSafetyReview } from ''

// TODO: Update the object below with actual values
const example = {
  "schemaVersion": null,
  "sourceId": null,
  "sourceVersionId": null,
  "extractionId": null,
  "reviewable": null,
  "canReview": null,
  "directControl": null,
  "samples": null,
  "decision": null,
} satisfies ImportSafetyReview

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ImportSafetyReview
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


