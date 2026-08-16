import { z } from "zod";

import { ResponseError } from "../../../contracts/generated/typescript";

const apiErrorSchema = z.object({
  code: z.string(),
  message: z.string().min(1),
  request_id: z.string().optional(),
});

export type GovernanceErrorDetail = {
  status: number;
  code: string | null;
  message: string | null;
};

// Generated clients intentionally throw ResponseError before decoding an error
// response. Parse a cloned body through Zod so governance UIs can explain a
// deterministic conflict without consuming the original response stream.
export async function readGovernanceError(
  error: unknown,
): Promise<GovernanceErrorDetail | null> {
  if (!(error instanceof ResponseError)) return null;
  try {
    const parsed = apiErrorSchema.safeParse(await error.response.clone().json());
    if (parsed.success) {
      return {
        status: error.response.status,
        code: parsed.data.code,
        message: humanizeDomainMessage(parsed.data.message),
      };
    }
  } catch {
    // Status-only fallback below also covers empty or non-JSON proxy errors.
  }
  return { status: error.response.status, code: null, message: null };
}

export function isIdentityConflictMessage(message: string | null): boolean {
  return Boolean(
    message?.includes("计划创建的页面或实体已存在") ||
      message?.includes("身份冲突"),
  );
}

function humanizeDomainMessage(message: string): string {
  return message.replace(/^(governance|page|knowledge):\s*/u, "").trim();
}
