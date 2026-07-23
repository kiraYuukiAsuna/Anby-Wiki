import * as Y from "yjs";

import { parseDocument, type Document } from "@/lib/ast/schema";

const ROOT_KEY = "ast";
const COLLABORATIVE_TEXT_FIELDS = new Set(["text", "display_text", "content"]);

type JSONValue =
  | null
  | boolean
  | number
  | string
  | JSONValue[]
  | { [key: string]: JSONValue };

/**
 * Creates a Yjs working copy from an AST v1 document.
 *
 * IDs and discriminators remain scalar values. User-editable text is stored as
 * Y.Text so concurrent character edits merge without changing stable AST IDs.
 */
export function createYjsAstDocument(document: Document): Y.Doc {
  const ydoc = new Y.Doc();
  syncYjsAst(ydoc, document, "initialize");
  return ydoc;
}

/** Initializes or reconciles a working copy from a validated AST document. */
export function syncYjsAst(
  ydoc: Y.Doc,
  document: Document,
  origin?: unknown,
): void {
  const root = getYjsAstRoot(ydoc);
  ydoc.transact(() => {
    reconcileMap(root, document as unknown as Record<string, JSONValue>);
  }, origin);
}

/** Returns the shared AST root for editor and synchronization adapters. */
export function getYjsAstRoot(ydoc: Y.Doc): Y.Map<unknown> {
  return ydoc.getMap<unknown>(ROOT_KEY);
}

/** Materializes and validates the current working copy as authoritative AST v1. */
export function materializeYjsAst(ydoc: Y.Doc): Document {
  return parseDocument(fromYValue(getYjsAstRoot(ydoc)));
}

export function encodeYjsState(ydoc: Y.Doc, stateVector?: Uint8Array): Uint8Array {
  return Y.encodeStateAsUpdate(ydoc, stateVector);
}

export function applyYjsState(ydoc: Y.Doc, update: Uint8Array): void {
  Y.applyUpdate(ydoc, update);
}

/**
 * Moves an AST item while preserving its stable business ID.
 *
 * Yjs shared types cannot be integrated twice, so a move clones the item's
 * materialized value before replacing its position in the shared array.
 */
export function moveYjsArrayItem(
  array: Y.Array<unknown>,
  from: number,
  to: number,
): void {
  if (from < 0 || from >= array.length || to < 0 || to >= array.length) {
    throw new RangeError("Yjs array move index is out of bounds");
  }
  if (from === to) {
    return;
  }
  const value = fromYValue(array.get(from)) as JSONValue;
  array.delete(from, 1);
  array.insert(to, [toYValue(value)]);
}

function toYValue(value: JSONValue, key?: string): unknown {
  if (typeof value === "string" && key && COLLABORATIVE_TEXT_FIELDS.has(key)) {
    const text = new Y.Text();
    text.insert(0, value);
    return text;
  }
  if (Array.isArray(value)) {
    const array = new Y.Array<unknown>();
    array.insert(
      0,
      value.map((item) => toYValue(item)),
    );
    return array;
  }
  if (value !== null && typeof value === "object") {
    const map = new Y.Map<unknown>();
    for (const [childKey, childValue] of Object.entries(value)) {
      map.set(childKey, toYValue(childValue, childKey));
    }
    return map;
  }
  return value;
}

function fromYValue(value: unknown): unknown {
  if (value instanceof Y.Text) {
    return value.toString();
  }
  if (value instanceof Y.Array) {
    return value.toArray().map(fromYValue);
  }
  if (value instanceof Y.Map) {
    return Object.fromEntries(
      Array.from(value.entries(), ([key, childValue]) => [
        key,
        fromYValue(childValue),
      ]),
    );
  }
  return value;
}

function reconcileMap(
  target: Y.Map<unknown>,
  desired: Record<string, JSONValue>,
): void {
  for (const key of Array.from(target.keys())) {
    if (!(key in desired)) {
      target.delete(key);
    }
  }
  for (const [key, value] of Object.entries(desired)) {
    const current = target.get(key);
    if (!reconcileValue(current, value, key)) {
      target.set(key, toYValue(value, key));
    }
  }
}

function reconcileValue(
  current: unknown,
  desired: JSONValue,
  key?: string,
): boolean {
  if (
    typeof desired === "string" &&
    key &&
    COLLABORATIVE_TEXT_FIELDS.has(key) &&
    current instanceof Y.Text
  ) {
    reconcileText(current, desired);
    return true;
  }
  if (Array.isArray(desired) && current instanceof Y.Array) {
    reconcileArray(current, desired);
    return true;
  }
  if (
    desired !== null &&
    !Array.isArray(desired) &&
    typeof desired === "object" &&
    current instanceof Y.Map
  ) {
    reconcileMap(current, desired);
    return true;
  }
  return Object.is(current, desired);
}

function reconcileText(target: Y.Text, desired: string): void {
  const current = target.toString();
  if (current === desired) return;
  let prefix = 0;
  while (
    prefix < current.length &&
    prefix < desired.length &&
    current[prefix] === desired[prefix]
  ) {
    prefix += 1;
  }
  let suffix = 0;
  while (
    suffix < current.length - prefix &&
    suffix < desired.length - prefix &&
    current[current.length - 1 - suffix] === desired[desired.length - 1 - suffix]
  ) {
    suffix += 1;
  }
  const deleteLength = current.length - prefix - suffix;
  if (deleteLength > 0) target.delete(prefix, deleteLength);
  const inserted = desired.slice(prefix, desired.length - suffix);
  if (inserted) target.insert(prefix, inserted);
}

function reconcileArray(target: Y.Array<unknown>, desired: JSONValue[]): void {
  if (isBlockArray(desired)) {
    reconcileBlockArray(target, desired);
    return;
  }
  for (let index = 0; index < desired.length; index += 1) {
    const value = desired[index];
    if (index >= target.length) {
      target.insert(index, [toYValue(value)]);
    } else if (!reconcileValue(target.get(index), value)) {
      target.delete(index, 1);
      target.insert(index, [toYValue(value)]);
    }
  }
  if (target.length > desired.length) {
    target.delete(desired.length, target.length - desired.length);
  }
}

function isBlockArray(
  values: JSONValue[],
): values is Array<Record<string, JSONValue> & { id: string }> {
  return values.every(
    (value) =>
      value !== null &&
      !Array.isArray(value) &&
      typeof value === "object" &&
      typeof value.id === "string",
  );
}

function reconcileBlockArray(
  target: Y.Array<unknown>,
  desired: Array<Record<string, JSONValue> & { id: string }>,
): void {
  for (let index = 0; index < desired.length; index += 1) {
    const block = desired[index];
    let currentIndex = findBlockIndex(target, block.id);
    if (currentIndex < 0) {
      target.insert(index, [toYValue(block)]);
      currentIndex = index;
    } else if (currentIndex !== index) {
      moveYjsArrayItem(target, currentIndex, index);
      currentIndex = index;
    }
    const current = target.get(currentIndex);
    if (current instanceof Y.Map) {
      reconcileMap(current, block);
    }
  }
  if (target.length > desired.length) {
    target.delete(desired.length, target.length - desired.length);
  }
}

function findBlockIndex(target: Y.Array<unknown>, id: string): number {
  return target.toArray().findIndex(
    (value) => value instanceof Y.Map && value.get("id") === id,
  );
}
