import { z } from "zod";

export const httpUrlSchema = z
  .url({ error: "请输入完整的 HTTP(S) 链接。" })
  .refine((value) => {
    try {
      const protocol = new URL(value).protocol;
      return protocol === "http:" || protocol === "https:";
    } catch {
      return false;
    }
  }, "请输入完整的 HTTP(S) 链接。");

export function safeHttpUrl(value: unknown): string | null {
  if (typeof value !== "string") return null;
  const parsed = httpUrlSchema.safeParse(value.trim());
  return parsed.success ? parsed.data : null;
}
