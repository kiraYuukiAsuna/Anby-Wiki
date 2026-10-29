import { z } from "zod";

import { ResponseError } from "../../../contracts/generated/typescript";

const apiErrorSchema = z.object({
  code: z.string(),
  message: z.string().min(1),
  request_id: z.string().optional(),
});

export async function importRequestError(error: unknown, fallback: string): Promise<string> {
  if (!(error instanceof ResponseError)) return fallback;
  try {
    const parsed = apiErrorSchema.safeParse(await error.response.clone().json());
    if (parsed.success) {
      const message = parsed.data.message.replace(/^(importer|evidence):\s*/u, "").trim();
      return parsed.data.request_id ? `${message}（请求 ID：${parsed.data.request_id}）` : message;
    }
  } catch {
    // Empty and non-JSON proxy errors still have a useful HTTP status.
  }
  if (error.response.status === 413) return "来源文件超过上传大小上限，请拆分后重试。";
  if (error.response.status === 429) return "上传请求过多，请稍后重试。";
  if (error.response.status === 403) return "当前账号没有执行此导入操作的权限。";
  if (error.response.status === 404) return "导入任务或来源不存在，可能已被移除。";
  if (error.response.status === 409) return "任务状态已变化，请刷新后再操作。";
  return `${fallback}（HTTP ${error.response.status}）`;
}
