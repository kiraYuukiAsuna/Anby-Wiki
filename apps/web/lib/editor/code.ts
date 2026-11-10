import type { Editor } from "@tiptap/core";
import { TextSelection } from "@tiptap/pm/state";
import { newBlockId } from "@/lib/editor/ids";

export function insertCodeBlock(editor: Editor) {
  if (!editor.isEditable) return false;
  const id = newBlockId();
  return editor.chain().focus().command(({ tr }) => {
    // The inserted block supplies the line boundary formerly provided by a soft break.
    const { $from } = tr.selection;
    if ($from.nodeAfter?.type.name === "hardBreak" || $from.nodeAfter?.text?.startsWith("\n")) tr.delete($from.pos, $from.pos + 1);
    if ($from.nodeBefore?.type.name === "hardBreak" || $from.nodeBefore?.text?.endsWith("\n")) tr.delete($from.pos - 1, $from.pos);
    return true;
  }).insertContent({ type: "codeBlock", attrs: { id } }).command(({ tr }) => {
    // Block insertion may leave the caret in the following paragraph.
    tr.doc.descendants((node, pos) => {
      if (node.attrs.id === id) tr.setSelection(TextSelection.create(tr.doc, pos + 1));
    });
    return true;
  }).run();
}
