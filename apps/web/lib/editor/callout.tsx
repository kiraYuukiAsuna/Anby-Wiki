import { useState } from "react";
import { Popover } from "radix-ui";
import { Info, OctagonAlert, TriangleAlert } from "lucide-react";
import { NodeViewContent, NodeViewWrapper, useEditorState, type ReactNodeViewProps } from "@tiptap/react";
import { Button } from "@/components/ui/button";

const kinds = [
  { value: "info", label: "信息", Icon: Info },
  { value: "warning", label: "警告", Icon: TriangleAlert },
  { value: "danger", label: "危险", Icon: OctagonAlert },
];
const colors = [
  ["default", "默认", "var(--muted)"], ["gray", "灰色", "#71717a"],
  ["brown", "棕色", "#78350f"], ["orange", "橙色", "#f97316"],
  ["yellow", "黄色", "#eab308"], ["green", "绿色", "#22c55e"],
  ["blue", "蓝色", "#3b82f6"], ["purple", "紫色", "#a855f7"],
  ["pink", "粉色", "#ec4899"], ["red", "红色", "#ef4444"],
];

export function CalloutView(props: ReactNodeViewProps & { onUpdateAttrs: (attrs: Record<string, unknown>) => boolean }) {
  const [open, setOpen] = useState(false);
  const editable = useEditorState({ editor: props.editor, selector: ({ editor }) => editor.isEditable });
  const { kind, color, icon, id } = props.node.attrs;
  const { Icon, label } = kinds.find(item => item.value === kind) ?? kinds[0];
  const iconContent = icon || <Icon size={18} aria-hidden />;
  return <NodeViewWrapper as="aside" className={`ast-callout ast-callout--${kind}`} data-editor-block-type="callout" data-id={id} data-callout-color={color} aria-label={label}>
    <div contentEditable={false} className="ast-callout__icon">
      {editable ? <Popover.Root open={open} onOpenChange={setOpen}>
        <Popover.Trigger asChild>
          <button type="button" className="ast-callout__style" aria-label="提示框样式" title="提示框样式">{iconContent}</button>
        </Popover.Trigger>
        <Popover.Portal><Popover.Content className="ast-callout-panel" align="start" sideOffset={8} collisionPadding={8} aria-label="提示框样式" onKeyDown={event => event.stopPropagation()}>
          <fieldset>
            <legend>类型</legend>
            <div className="flex gap-2">{kinds.map(item => <Button key={item.value} type="button" size="sm" variant={kind === item.value ? "secondary" : "ghost"} aria-pressed={kind === item.value} onClick={() => props.onUpdateAttrs({ kind: item.value })}><item.Icon aria-hidden />{item.label}</Button>)}</div>
          </fieldset>
          <fieldset>
            <legend>颜色</legend>
            <div className="grid grid-cols-5 gap-2">{colors.map(([value, name, swatch]) => <button key={value} type="button" className="ast-callout-swatch" aria-label={name} title={name} aria-pressed={(color || "default") === value} onClick={() => props.onUpdateAttrs({ color: value })}><span style={{ background: swatch }} /></button>)}</div>
          </fieldset>
          <fieldset>
            <legend>图标</legend>
            <div className="flex flex-wrap gap-1">
              <Button type="button" size="sm" variant="ghost" aria-pressed={!icon} onClick={() => props.onUpdateAttrs({ icon: "" })}>默认</Button>
              {["💡", "📌", "📝", "✅", "⚠️"].map(value => <Button key={value} type="button" size="icon-sm" variant="ghost" aria-label={`图标 ${value}`} aria-pressed={icon === value} onClick={() => props.onUpdateAttrs({ icon: value })}>{value}</Button>)}
            </div>
            <form key={icon} className="mt-2 flex gap-2" onSubmit={event => {
              event.preventDefault();
              const value = String(new FormData(event.currentTarget).get("icon") ?? "").trim();
              if (props.onUpdateAttrs({ icon: value })) setOpen(false);
            }}>
              <input name="icon" aria-label="自定义图标" placeholder="自定义图标" defaultValue={icon} maxLength={64} className="ast-editable-block__control min-w-0 flex-1" />
              <Button type="submit" size="sm" variant="outline">应用</Button>
            </form>
          </fieldset>
        </Popover.Content></Popover.Portal>
      </Popover.Root> : <span className="ast-callout__style">{iconContent}</span>}
    </div>
    <NodeViewContent className="ast-callout__content" />
  </NodeViewWrapper>;
}
