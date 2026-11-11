"use client";

import { useState } from "react";
import dynamic from "next/dynamic";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ResourcePicker } from "@/components/editor/resource-picker";
import { buttonBlockSchema, type ButtonAction } from "@/lib/ast/schema";
import { newBlockId } from "@/lib/editor/ids";

const BlockEditor = dynamic(() => import("@/components/editor/block-editor").then(module => module.BlockEditor), { ssr: false });
type ButtonValue = { label: string; confirmation?: string; actions: ButtonAction[] };
const actionNames = { notification: "显示通知", insert_blocks: "插入内容", webhook: "发送 Webhook", update_dataset_record: "旧版数据集动作" };
const actionRow = (action: ButtonAction) => ({ key: newBlockId(), action, payload: action.type === "webhook" ? JSON.stringify(action.payload, null, 2) : "" });

export function PageButtonInsertDialog({ open, onOpenChange, currentPageId, initialValue, onInsert }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentPageId?: string;
  initialValue?: ButtonValue;
  onInsert: (value: ButtonValue) => boolean | void;
}) {
  const [label, setLabel] = useState(initialValue?.label ?? "运行操作");
  const [confirmation, setConfirmation] = useState(initialValue?.confirmation ?? (initialValue ? "" : "确定执行这个页面按钮吗？"));
  const [rows, setRows] = useState(() => (initialValue?.actions ?? [{ type: "notification", message: "", tone: "success" } as ButtonAction]).map(actionRow));
  const update = (key: string, action: ButtonAction) => setRows(current => current.map(row => row.key === key ? { ...row, action } : row));
  const add = (type: "notification" | "insert_blocks" | "webhook") => setRows(current => [...current, actionRow(type === "notification" ? { type, message: "", tone: "success" } : type === "webhook" ? { type, url: "", payload: {} } : { type, target_page_id: currentPageId ?? "", blocks: [{ id: newBlockId(), type: "paragraph", content: [] }] })]);
  const submit = () => {
    let actions: ButtonAction[];
    try {
      actions = rows.map(({ action, payload }) => action.type === "webhook" ? { ...action, payload: JSON.parse(payload) } : action);
    } catch { toast.error("请求内容需为有效的 JSON 对象"); return; }
    const parsed = buttonBlockSchema.safeParse({ id: newBlockId(), type: "button", label: label.trim(), ...(confirmation.trim() ? { confirmation: confirmation.trim() } : {}), actions });
    if (!parsed.success) { toast.error("按钮配置不完整，请检查名称和动作", { description: parsed.error.issues[0]?.message }); return; }
    if (onInsert({ label: parsed.data.label, confirmation: parsed.data.confirmation, actions: parsed.data.actions }) !== false) onOpenChange(false);
  };

  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl" onKeyDown={event => event.stopPropagation()}>
      <DialogHeader><DialogTitle>{initialValue ? "编辑页面按钮" : "插入页面按钮"}</DialogTitle><DialogDescription>读者点击并确认后才会执行操作。插入内容需要先提交审核。</DialogDescription></DialogHeader>
      <Label className="grid gap-2">按钮名称<Input maxLength={120} value={label} onChange={event => setLabel(event.target.value)} /></Label>
      <Label className="grid gap-2">确认提示（可选）<Input maxLength={500} value={confirmation} onChange={event => setConfirmation(event.target.value)} /></Label>
      {rows.map(({ key, action, payload }, index) => <section key={key} aria-label={`动作 ${index + 1}`} className="min-w-0 space-y-3 rounded-lg border p-3">
        <div className="flex items-center justify-between gap-2"><span className="text-sm font-medium">{index + 1}. {actionNames[action.type]}</span><Button type="button" variant="ghost" size="sm" disabled={rows.length === 1} onClick={() => setRows(current => current.filter(row => row.key !== key))}>移除动作</Button></div>
        {action.type === "insert_blocks" ? <>
          <div className="space-y-2"><Label className="grid gap-2">目标页面</Label><ResourcePicker kind="page" value={action.target_page_id} onChange={choice => update(key, { ...action, target_page_id: choice.id })} /></div>
          <Label className="grid gap-2">插入内容</Label><BlockEditor initialAst={{ schema_version: 1, type: "document", children: action.blocks }} onChange={doc => update(key, { ...action, blocks: doc.children })} />
        </> : action.type === "notification" ? <>
          <Label className="grid gap-2">通知内容<Input maxLength={500} value={action.message} onChange={event => update(key, { ...action, message: event.target.value })} /></Label>
          <Label className="grid gap-2">通知级别<select className="h-9 w-full rounded-lg border bg-background px-2" value={action.tone ?? "default"} onChange={event => update(key, { ...action, tone: event.target.value as typeof action.tone })}>{["default", "success", "info", "warning", "error"].map((tone, i) => <option key={tone} value={tone}>{["默认", "成功", "信息", "警告", "错误"][i]}</option>)}</select></Label>
        </> : action.type === "webhook" ? <>
          <Label className="grid gap-2">公网 HTTPS URL<Input type="url" value={action.url} onChange={event => update(key, { ...action, url: event.target.value })} /></Label>
          <Label className="grid gap-2">请求内容（JSON）<Textarea className="min-h-32 font-mono" spellCheck={false} value={payload} onChange={event => { const value = event.target.value; setRows(current => current.map(row => row.key === key ? { ...row, payload: value } : row)); }} /></Label>
        </> : <p className="text-sm text-muted-foreground">此旧版数据集动作已停用，原配置保留。</p>}
      </section>)}
      <div className="flex flex-wrap gap-2">{(["notification", "insert_blocks", "webhook"] as const).map(type => <Button key={type} type="button" variant="outline" size="sm" disabled={rows.length >= 10} onClick={() => add(type)}>{type === "notification" ? "添加通知" : type === "insert_blocks" ? "添加内容操作" : "添加 Webhook"}</Button>)}</div>
      <DialogFooter><Button type="button" variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button type="button" onClick={submit}>{initialValue ? "保存按钮" : "插入按钮"}</Button></DialogFooter>
    </DialogContent>
  </Dialog>;
}
