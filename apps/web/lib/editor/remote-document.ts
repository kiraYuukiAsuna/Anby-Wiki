import type { Node } from "@tiptap/pm/model";
import type { Transaction } from "@tiptap/pm/state";

/** Preserve unchanged ranges and keep remote edits out of local undo history. */
export function applyRemoteDocument(transaction: Transaction, next: Node): void {
  const start = transaction.doc.content.findDiffStart(next.content);
  if (start === null) return;
  const end = transaction.doc.content.findDiffEnd(next.content)!;
  const overlap = start - Math.min(end.a, end.b);
  if (overlap > 0) { end.a += overlap; end.b += overlap; }
  transaction.replace(start, end.a, next.slice(start, end.b));
  transaction.setMeta("addToHistory", false);
}
