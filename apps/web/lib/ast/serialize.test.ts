import * as fs from "node:fs";
import * as path from "node:path";

import { describe, expect, it } from "vitest";

import { contentHash } from "./hash";
import { parseDocument } from "./schema";
import { canonicalJson } from "./serialize";

// vitest 以 apps/web 为 cwd 运行，canonical fixtures 与 Go 测试共享同一目录。
const canonicalDir = path.resolve(
  process.cwd(),
  "../../contracts/schemas/ast/v1/fixtures/canonical",
);

function listCanonicalFixtures(): string[] {
  const names = fs
    .readdirSync(canonicalDir)
    .filter((f) => f.endsWith(".json") && !f.endsWith(".canonical.json"))
    .sort();
  if (names.length === 0) {
    throw new Error("fixtures/canonical 目录为空");
  }
  return names;
}

describe("AST v1 canonical 共享 fixtures（与 Go 侧一致性）", () => {
  for (const name of listCanonicalFixtures()) {
    const base = name.replace(/\.json$/, "");
    const inputRaw = fs.readFileSync(path.join(canonicalDir, name), "utf8");
    const wantCanon = fs.readFileSync(
      path.join(canonicalDir, `${base}.canonical.json`),
      "utf8",
    );
    const wantHash = fs
      .readFileSync(path.join(canonicalDir, `${base}.sha256`), "utf8")
      .trim();

    it(`canonical: ${name}`, () => {
      const parsed: unknown = JSON.parse(inputRaw);
      // 与 Go CanonicalizeJSON(原始字节) 一致。
      expect(canonicalJson(parsed)).toBe(wantCanon);
      // 与 Go ContentHash(doc) 一致；Zod parse 后（字段重排）哈希不变。
      const doc = parseDocument(parsed);
      expect(canonicalJson(doc)).toBe(wantCanon);
      expect(contentHash(doc)).toBe(wantHash);
      expect(contentHash(parsed)).toBe(wantHash);
      // canonical 幂等。
      expect(canonicalJson(JSON.parse(wantCanon))).toBe(wantCanon);
    });
  }
});

describe("canonicalJson 规则", () => {
  it("键序/空白无关：语义相同字面不同的对象 canonical 相同", () => {
    const a = { b: 1, a: [true, null, "x"], c: { y: 2, x: 1 } };
    const b = { c: { x: 1, y: 2 }, a: [true, null, "x"], b: 1 };
    expect(canonicalJson(a)).toBe(canonicalJson(b));
    expect(canonicalJson(a)).toBe('{"a":[true,null,"x"],"b":1,"c":{"x":1,"y":2}}');
    expect(contentHash(a)).toBe(contentHash(b));
  });

  it("字符串转义：仅引号/反斜杠/控制字符转义，非 ASCII 与 <>& 保留", () => {
    const ctrl = String.fromCharCode(1); // U+0001，canonical 输出 \u0001
    const s = `<tag> & "引号" \\ 换行\n制表\t 退格\b 换页\f 回车\r ${ctrl} 汉字 👩‍💻`;
    expect(canonicalJson({ s })).toBe(
      '{"s":"<tag> & \\"引号\\" \\\\ 换行\\n制表\\t 退格\\b 换页\\f 回车\\r \\u0001 汉字 👩‍💻"}',
    );
  });

  it("数字：整数规范化，非整数/超范围抛错", () => {
    expect(canonicalJson({ a: 1.0, b: -0, c: 100 })).toBe(
      '{"a":1,"b":0,"c":100}',
    );
    expect(() => canonicalJson({ a: 1.5 })).toThrow();
    expect(() => canonicalJson({ a: 2 ** 53 })).toThrow();
  });

  it("undefined 值按 JSON 语义忽略", () => {
    expect(canonicalJson({ a: undefined, b: 1 })).toBe('{"b":1}');
  });
});
