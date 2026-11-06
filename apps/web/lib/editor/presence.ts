import { Extension } from "@tiptap/core";
import type { Node, ResolvedPos } from "@tiptap/pm/model";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

import type { EditorCursor, EditorPosition } from "@/lib/collaboration/presence";
import { createAdapterState, resolveBlockId, type AdapterState } from "@/lib/editor/ids";

export interface EditorPeer {
  clientId: string;
  label: string;
  cursor: EditorCursor;
}

interface PresenceOptions {
  adapter: AdapterState;
}

const presenceKey = new PluginKey("anbywiki-presence");
const COLORS = ["#2563eb", "#9333ea", "#0891b2", "#c2410c", "#15803d"];
const peersByAdapter = new WeakMap<AdapterState, EditorPeer[]>();

export function setPresencePeers(adapter: AdapterState, peers: EditorPeer[]): void {
  peersByAdapter.set(adapter, peers);
}

function astId(id: string, adapter: AdapterState): string {
  return resolveBlockId(id, adapter);
}

export function editorPosition(position: ResolvedPos, adapter: AdapterState): EditorPosition | null {
  if (!position.parent.isTextblock) return null;
  for (let depth = position.depth; depth > 0; depth -= 1) {
    const id = position.node(depth).attrs.id;
    if (typeof id === "string") return { blockId: astId(id, adapter), offset: position.parentOffset };
  }
  return null;
}

export function resolveEditorPosition(doc: Node, position: EditorPosition, adapter: AdapterState): number | null {
  let result: number | null = null;
  doc.descendants((node, start) => {
    if (result !== null) return false;
    if (typeof node.attrs.id !== "string" || astId(node.attrs.id, adapter) !== position.blockId) return;
    if (node.isTextblock) {
      result = start + 1 + Math.min(position.offset, node.content.size);
      return false;
    }
    node.descendants((child, offset) => {
      if (result !== null) return false;
      if (child.isTextblock) {
        result = start + 1 + offset + 1 + Math.min(position.offset, child.content.size);
        return false;
      }
    });
    return false;
  });
  return result;
}

export const PresenceExtension = Extension.create<PresenceOptions>({
  name: "anbywikiPresence",
  addOptions() {
    return { adapter: createAdapterState() };
  },
  addProseMirrorPlugins() {
    const adapter = this.options.adapter;
    return [new Plugin({
      key: presenceKey,
      props: {
        decorations(state) {
          const decorations: Decoration[] = [];
          for (const peer of peersByAdapter.get(adapter) ?? []) {
            const anchor = resolveEditorPosition(state.doc, peer.cursor.anchor, adapter);
            const head = resolveEditorPosition(state.doc, peer.cursor.head, adapter);
            if (anchor === null || head === null) continue;
            const hash = Array.from(peer.clientId).reduce((sum, char) => sum + char.charCodeAt(0), 0);
            const color = COLORS[hash % COLORS.length];
            if (anchor !== head) {
              decorations.push(Decoration.inline(Math.min(anchor, head), Math.max(anchor, head), {
                style: `background-color:${color}25`,
                "data-collaboration-selection": peer.clientId,
              }));
            }
            decorations.push(Decoration.widget(head, () => {
              const caret = document.createElement("span");
              caret.className = "collaboration-caret";
              caret.style.setProperty("--peer-color", color);
              caret.dataset.collaborationCaret = peer.clientId;
              caret.contentEditable = "false";
              caret.setAttribute("aria-label", `${peer.label} 的光标`);
              const label = document.createElement("span");
              label.className = "collaboration-caret-label";
              label.textContent = peer.label;
              caret.append(label);
              return caret;
            }, { key: `${peer.clientId}:${peer.label}`, side: 1 }));
          }
          return DecorationSet.create(state.doc, decorations);
        },
      },
    })];
  },
});
