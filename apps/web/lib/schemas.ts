import { z } from "zod";

/**
 * Zod 运行时校验占位示例。
 * 后端 DTO 以 OpenAPI 契约为准（M0-T06 生成客户端），此文件只存放
 * 前端自有的运行时校验 schema（表单、环境变量、localStorage 数据等）。
 */
export const paginationQuerySchema = z.object({
  page: z.coerce.number().int().min(1).default(1),
  pageSize: z.coerce.number().int().min(1).max(100).default(20),
});

export type PaginationQuery = z.infer<typeof paginationQuerySchema>;
