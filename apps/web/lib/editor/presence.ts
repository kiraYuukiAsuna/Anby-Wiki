import { createExtension, createStore, type ExtensionOptions } from "@blocknote/core";
import type { Node, ResolvedPos } from "@tiptap/pm/model";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

import type { EditorCursor, EditorPosition } from "@/lib/collaboration/presence";
import { resolveBlockId, type AdapterState } from "@/lib/editor/ids";

export interface EditorPeer {
  clientId: string;
  label: string;
  cursor: EditorCursor;
}

const presenceKey = new PluginKey("anbywiki-presence");
const COLORS = ["#2563eb", "#9333ea", "#0891b2", "#c2410c", "#15803d"];

function astId(id: string, adapter: AdapterState): string {
  return adapter.liftedParagraphIds.get(id) ?? resolveBlockId(id, adapter);
}

/** Handles UUID remapping and the lifted quote/list paragraph adapter. */
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

export const PresenceExtension = createExtension(({ editor, options }: ExtensionOptions<{ adapter: AdapterState }>) => {
  const store = createStore({ peers: [] as EditorPeer[] }, {
    onUpdate: () => editor.transact((transaction) => transaction.setMeta(presenceKey, true)),
  });
  return {
    key: "anbywikiPresence" as const,
    store,
    prosemirrorPlugins: [new Plugin({
      key: presenceKey,
      props: {
        decorations(state) {
          const decorations: Decoration[] = [];
          for (const peer of store.state.peers) {
            const anchor = resolveEditorPosition(state.doc, peer.cursor.anchor, options.adapter);
            const head = resolveEditorPosition(state.doc, peer.cursor.head, options.adapter);
            if (anchor === null || head === null) continue;
            const hash = Array.from(peer.clientId).reduce((sum, char) => sum + char.charCodeAt(0), 0);
            const color = COLORS[hash % COLORS.length];
            if (anchor !== head) decorations.push(Decoration.inline(Math.min(anchor, head), Math.max(anchor, head), {
              style: `background-color:${color}25`,
              "data-collaboration-selection": peer.clientId,
            }));
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
    })],
    setPeers: (peers: EditorPeer[]) => store.setState({ peers }),
  };
});
