import { describe, expect, it } from "vitest";

import { httpUrlSchema, safeHttpUrl } from "./http-url";

describe("httpUrlSchema", () => {
  it.each([
    "javascript:alert(1)",
    "data:text/html,<script>alert(1)</script>",
    "file:///etc/passwd",
  ])("拒绝危险协议 %s", (value) => {
    expect(httpUrlSchema.safeParse(value).success).toBe(false);
    expect(safeHttpUrl(value)).toBeNull();
  });

  it.each(["http://example.com/a", "https://example.com/a?b=1#c"])(
    "接受 http/https %s",
    (value) => {
      expect(httpUrlSchema.parse(value)).toBe(value);
      expect(safeHttpUrl(` ${value} `)).toBe(value);
    },
  );
});
