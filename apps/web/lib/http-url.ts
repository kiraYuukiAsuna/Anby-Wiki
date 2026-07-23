import { z } from "zod";

export const httpUrlSchema = z
  .url({ error: "链接必须是合法的 http/https URL" })
  .refine((value) => {
    try {
      const protocol = new URL(value).protocol;
      return protocol === "http:" || protocol === "https:";
    } catch {
      return false;
    }
  }, "链接必须是合法的 http/https URL");

export function safeHttpUrl(value: unknown): string | null {
  if (typeof value !== "string") return null;
  const parsed = httpUrlSchema.safeParse(value.trim());
  return parsed.success ? parsed.data : null;
}
