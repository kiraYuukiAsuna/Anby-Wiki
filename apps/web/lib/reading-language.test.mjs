import assert from "node:assert/strict";
import { test } from "node:test";
import { preferredPageVariant } from "./reading-language.ts";

const items = [
  { pageId: "zh", language: "zh-Hans", title: "中文" },
  { pageId: "en", language: "en", title: "English" },
];
test("reading preference selects an available translation", () => {
  assert.equal(preferredPageVariant(items, "zh", "en")?.pageId, "en");
  assert.equal(preferredPageVariant(items, "en", "zh-Hans")?.pageId, "zh");
  assert.equal(preferredPageVariant(items.slice(0, 1), "zh", "en"), undefined);
});
test("a translated page redirecting back to its source terminates navigation", () => {
  const first = preferredPageVariant(items, "zh", "en");
  assert.equal(first?.pageId, "en");
  // GET en resolves back to zh; the next render must not send the user back to en.
  assert.equal(preferredPageVariant(items, "zh", "en", first.pageId), undefined);
  assert.equal(preferredPageVariant(items, "en", "zh-Hans", "zh"), undefined);
});
