"use client";

import { useState } from "react";
import dynamic from "next/dynamic";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ResourcePicker, type ResourceChoice } from "@/components/editor/resource-picker";
import { type ButtonAction, type BreadcrumbItem } from "@/lib/ast/schema";

const BlockEditor = dynamic(() => import("@/components/editor/block-editor").then(module => module.BlockEditor), { ssr: false });

export function AnchorPicker({ pageId, headingId, onChange }: { pageId: string; headingId: string; onChange: (choice: ResourceChoice) => boolean }) {
  const [open, setOpen] = useState(false);
  const [page, setPage] = useState(pageId);
  const [heading, setHeading] = useState<ResourceChoice>({ id: headingId, label: "" });
  return <>
    <Button type="button" variant="outline" onClick={() => { setPage(pageId); setHeading({ id: headingId, label: "" }); setOpen(true); }}>选择页面与章节</Button>
    <Dialog open={open} onOpenChange={setOpen}><DialogContent onKeyDown={event => event.stopPropagation()}><DialogHeader><DialogTitle>选择章节链接</DialogTitle><DialogDescription className="sr-only">先选择页面，再选择章节。</DialogDescription></DialogHeader>
      <ResourcePicker kind="page" value={page} onChange={item => { setPage(item.id); setHeading({ id: "", label: "" }); }} />
      <ResourcePicker kind="heading" value={heading.id} pageId={page} valueLabel={heading.label} onChange={setHeading} />
      <Button type="button" disabled={!page || !heading.id} onClick={() => { if (onChange({ ...heading, pageId: page })) setOpen(false); }}>保存章节链接</Button>
    </DialogContent></Dialog>
  </>;
}

export function BreadcrumbFields({ value, onChange }: { value: BreadcrumbItem[]; onChange: (value: BreadcrumbItem[]) => boolean }) {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState(value);
  return <>
    <Button type="button" variant="outline" onClick={() => { setItems(value); setOpen(true); }}>编辑页面路径</Button>
    <Dialog open={open} onOpenChange={setOpen}><DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-xl" onKeyDown={event => event.stopPropagation()}><DialogHeader><DialogTitle>页面路径</DialogTitle><DialogDescription className="sr-only">按顺序选择路径中的页面，可保留无链接的文字。</DialogDescription></DialogHeader>
      {items.map((item, index) => <div key={index} className="space-y-2 rounded-lg border p-3">
        <Label>显示名称<Input value={item.display_text} onChange={event => setItems(items.map((old, i) => i === index ? { ...old, display_text: event.target.value } : old))} /></Label>
        <ResourcePicker kind="page" value={item.target_page_id ?? ""} clearable onChange={choice => setItems(items.map((old, i) => i === index ? { display_text: choice.label || old.display_text, ...(choice.id ? { target_page_id: choice.id } : {}) } : old))} />
        <Button type="button" variant="ghost" disabled={items.length === 1} onClick={() => setItems(items.filter((_, i) => i !== index))}>移除路径项</Button>
      </div>)}
      <Button type="button" variant="outline" disabled={items.length >= 32} onClick={() => setItems([...items, { display_text: "当前页面" }])}>添加路径项</Button>
      <Button type="button" onClick={() => { if (onChange(items)) setOpen(false); }}>保存页面路径</Button>
    </DialogContent></Dialog>
  </>;
}

export function ButtonActionsFields({ value, onChange }: { value: ButtonAction[]; onChange: (value: ButtonAction[]) => boolean }) {
  const [open, setOpen] = useState(false);
  const [actions, setActions] = useState(value);
  const update = (index: number, action: ButtonAction) => setActions(actions.map((old, i) => i === index ? action : old));
  return <>
    <Button type="button" variant="outline" onClick={() => { setActions(value); setOpen(true); }}>编辑按钮动作</Button>
    <Dialog open={open} onOpenChange={setOpen}><DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl" onKeyDown={event => event.stopPropagation()}><DialogHeader><DialogTitle>按钮动作</DialogTitle><DialogDescription>读者点击并确认后执行。</DialogDescription></DialogHeader>
      {actions.map((action, index) => <section key={index} className="min-w-0 space-y-3 rounded-lg border p-3">
        {action.type === "insert_blocks" ? <>
          <Label>目标页面</Label><ResourcePicker kind="page" value={action.target_page_id} onChange={choice => update(index, { ...action, target_page_id: choice.id })} />
          <Label>插入内容</Label><BlockEditor initialAst={{ schema_version: 1, type: "document", children: action.blocks }} onChange={doc => update(index, { ...action, blocks: doc.children })} />
        </> : action.type === "notification" ? <>
          <Label>通知内容<Input value={action.message} onChange={event => update(index, { ...action, message: event.target.value })} /></Label>
          <Label>通知级别<select className="h-9 w-full rounded-lg border bg-background px-2" value={action.tone ?? "default"} onChange={event => update(index, { ...action, tone: event.target.value as typeof action.tone })}>{["default", "success", "info", "warning", "error"].map((tone, i) => <option key={tone} value={tone}>{["默认", "成功", "信息", "警告", "错误"][i]}</option>)}</select></Label>
        </> : action.type === "webhook" ? <>
          <Label>公网 HTTPS URL<Input value={action.url} onChange={event => update(index, { ...action, url: event.target.value })} /></Label>
          <Label>请求内容（JSON）<Textarea defaultValue={JSON.stringify(action.payload, null, 2)} onBlur={event => { try { const payload: unknown = JSON.parse(event.target.value); if (payload && typeof payload === "object" && !Array.isArray(payload)) update(index, { ...action, payload: payload as Record<string, unknown> }); else throw new Error("invalid payload"); } catch { event.target.value = JSON.stringify(action.payload, null, 2); toast.error("请求内容需为 JSON 对象，原内容已保留。"); } }} /></Label>
        </> : <p className="text-sm text-muted-foreground">此旧版数据集动作已停用，原配置保留。</p>}
        <Button type="button" variant="ghost" disabled={actions.length === 1} onClick={() => setActions(actions.filter((_, i) => i !== index))}>移除动作</Button>
      </section>)}
      <Button type="button" onClick={() => { if (onChange(actions)) setOpen(false); }}>保存按钮动作</Button>
    </DialogContent></Dialog>
  </>;
}
