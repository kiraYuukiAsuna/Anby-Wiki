// Editor Adapter 错误类型（M2-T02 / ADR-0005 验收标准 3：显式拒绝，不静默丢失）。

/** Adapter 错误基类。 */
export class EditorAdapterError extends Error {
  override name = "EditorAdapterError";
}

/**
 * BlockNote 文档含有 AST v1 无法无损映射的特性（未映射的块类型、
 * 文本样式、非默认 prop 等）时抛出。消息必须包含块位置与特性名。
 */
export class UnsupportedBlockNoteFeatureError extends EditorAdapterError {
  override name = "UnsupportedBlockNoteFeatureError";
}

/** toAst 产物未通过 parseDocument（AST v1 Zod 校验）时抛出，附 Zod issue 路径。 */
export class AstValidationError extends EditorAdapterError {
  override name = "AstValidationError";
}
