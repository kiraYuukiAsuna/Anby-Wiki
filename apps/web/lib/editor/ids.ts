// Block ID 生成与适配会话状态（M2-T02 / ADR-0005、ADR-0008）。
//
// AST v1 要求 Block ID 为 UUIDv7。BlockNote 0.52 内部用 lib0 的 uuidv4 给
// 新建块分配 ID（见 @blocknote/core UniqueID 扩展），因此 Adapter 在 toAst
// 边界把「非 UUIDv7 的 BlockNote ID」重映射为 UUIDv7；映射关系保存在
// AdapterState 中，保证同一编辑会话内对同一 BlockNote 块重复序列化时 ID 稳定。
import { v7 as uuidv7, validate as uuidValidate, version as uuidVersion } from "uuid";

/** 生成一个新的 Block ID（UUIDv7，小写连字符形式）。 */
export function newBlockId(): string {
  return uuidv7();
}

/** 判断字符串是否为 UUIDv7。 */
export function isUuidV7(id: string): boolean {
  if (!uuidValidate(id)) return false;
  try {
    return uuidVersion(id) === 7;
  } catch {
    return false;
  }
}

/**
 * Adapter 会话状态：随编辑器实例存活（受控组件用 useRef 持有）。
 * fromAst 与 toAst 必须共享同一个 state，ID 稳定性才成立。
 */
export interface AdapterState {
  /** BlockNote 块 ID（非 UUIDv7，即编辑器内新建块）→ 分配的 UUIDv7。 */
  generatedIds: Map<string, string>;
  /**
   * 内容提升（lift）映射：AST 容器（quote / list_item）首个 paragraph 子块
   * 会被提升为 BlockNote 块的行内 content，其 AST ID 记在这里
   * （key = BlockNote 块 ID），toAst 还原时取回。
   */
  liftedParagraphIds: Map<string, string>;
  /** AST list 容器 ID 映射：key = list_item 的块 ID，value = list 容器 UUIDv7。 */
  listIds: Map<string, string>;
}

export function createAdapterState(): AdapterState {
  return {
    generatedIds: new Map(),
    liftedParagraphIds: new Map(),
    listIds: new Map(),
  };
}

/**
 * 把 BlockNote 块 ID 解析为 AST Block ID：
 * 已是 UUIDv7（来自 fromAst 或上一轮 toAst）则原样保留；
 * 否则（BlockNote 给新建块分配的 uuidv4 等）经 state 稳定地分配一个 UUIDv7。
 */
export function resolveBlockId(bnId: string, state: AdapterState): string {
  if (isUuidV7(bnId)) return bnId;
  let id = state.generatedIds.get(bnId);
  if (!id) {
    id = newBlockId();
    state.generatedIds.set(bnId, id);
  }
  return id;
}
