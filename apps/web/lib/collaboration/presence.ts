import * as Y from "yjs";
import { z } from "zod";

import { getYjsAstRoot } from "@/lib/collaboration/yjs-ast";

export interface EditorPosition {
  blockId: string;
  /** UTF-16 code units; inline atoms occupy one position, as in ProseMirror. */
  offset: number;
}

export interface EditorCursor {
  anchor: EditorPosition;
  head: EditorPosition;
}

const positionSchema = z.object({
  block_id: z.string().uuid(),
  offset: z.number().int().min(0).max(1 << 20),
  relative_position: z.string().max(344).optional(),
});
const cursorSchema = z.object({
  version: z.literal(1),
  anchor: positionSchema,
  head: positionSchema,
});

export interface PresenceCursor {
  version: 1;
  block_id: string;
  selection: "text";
  anchor: z.infer<typeof positionSchema>;
  head: z.infer<typeof positionSchema>;
}

/** Resolve stable business blocks recursively, including table cells/lists. */
function findBlock(value: unknown, blockId: string): Y.Map<unknown> | null {
  if (value instanceof Y.Map) {
    if (value.get("id") === blockId) return value;
    return findBlock(value.get("children"), blockId);
  }
  if (value instanceof Y.Array) {
    for (const child of value.toArray()) {
      const found = findBlock(child, blockId);
      if (found) return found;
    }
  }
  return null;
}

function runs(block: Y.Map<unknown>): Array<Y.Text | number> {
  const content = block.get("content") ?? block.get("text");
  if (content instanceof Y.Text) return [content];
  if (!(content instanceof Y.Array)) return [];
  return content.toArray().map((inline) => {
    if (!(inline instanceof Y.Map)) return 1;
    const type = inline.get("type");
    const text = inline.get(type === "external_link" ? "display_text" : "text");
    return (type === "text" || type === "inline_code" || type === "external_link") &&
      text instanceof Y.Text ? text : 1;
  });
}

export function encodePresencePosition(ydoc: Y.Doc, position: EditorPosition): z.infer<typeof positionSchema> {
  const result: z.infer<typeof positionSchema> = {
    block_id: position.blockId,
    offset: position.offset,
  };
  const block = findBlock(getYjsAstRoot(ydoc), position.blockId);
  if (!block) return result;
  let offset = position.offset;
  for (const run of runs(block)) {
    const length = typeof run === "number" ? run : run.length;
    if (run instanceof Y.Text && offset <= length) {
      const bytes = Y.encodeRelativePosition(Y.createRelativePositionFromTypeIndex(run, offset));
      result.relative_position = btoa(String.fromCharCode(...bytes));
      break;
    }
    offset -= length;
  }
  return result;
}

export function resolvePresencePosition(ydoc: Y.Doc, cursor: unknown): EditorCursor | null {
  const parsed = cursorSchema.safeParse(cursor);
  if (!parsed.success) return null;
  const resolve = (position: z.infer<typeof positionSchema>): EditorPosition | null => {
    const block = findBlock(getYjsAstRoot(ydoc), position.block_id);
    if (!block) return null;
    const content = runs(block);
    if (position.relative_position) {
      try {
        const bytes = Uint8Array.from(atob(position.relative_position), (char) => char.charCodeAt(0));
        const absolute = Y.createAbsolutePositionFromRelativePosition(Y.decodeRelativePosition(bytes), ydoc);
        let offset = 0;
        for (const run of content) {
          if (absolute && run === absolute.type) {
            return { blockId: position.block_id, offset: offset + absolute.index };
          }
          offset += typeof run === "number" ? run : run.length;
        }
        // Deleted/replaced text types must not point into unrelated content.
        return null;
      } catch {
        return null;
      }
    }
    const length = content.reduce<number>((sum, run) => sum + (typeof run === "number" ? run : run.length), 0);
    return { blockId: position.block_id, offset: Math.min(position.offset, length) };
  };
  const anchor = resolve(parsed.data.anchor);
  const head = resolve(parsed.data.head);
  return anchor && head ? { anchor, head } : null;
}
