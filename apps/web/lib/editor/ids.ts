// Block ID generation and editor session state.
//
// AST v1 requires UUIDv7 Block IDs. Tiptap nodes normally receive IDs through
// the BlockIds extension; the adapter keeps a stable UUIDv7 fallback for any
// transient editor node observed before that plugin writes an ID.
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

/** Adapter session state, scoped to a single editor instance. */
export interface AdapterState {
  /** Editor node ID -> assigned AST UUIDv7 fallback. */
  generatedIds: Map<string, string>;
}

export function createAdapterState(): AdapterState {
  return { generatedIds: new Map() };
}

/** Resolve an editor node ID to an AST Block ID. */
export function resolveBlockId(editorNodeId: string, state: AdapterState): string {
  if (isUuidV7(editorNodeId)) return editorNodeId;
  let id = state.generatedIds.get(editorNodeId);
  if (!id) {
    id = newBlockId();
    state.generatedIds.set(editorNodeId, id);
  }
  return id;
}
