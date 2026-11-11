"use client";

import { useId, useRef, useState } from "react";
import { ArrowUp, ArrowDown, ChevronRight, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ResourcePicker, type ResourceChoice } from "@/components/editor/resource-picker";
import { type BreadcrumbItem } from "@/lib/ast/schema";

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

export function BreadcrumbFields({ value, onChange, editable = true }: { value: BreadcrumbItem[]; onChange: (value: BreadcrumbItem[]) => boolean; editable?: boolean }) {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState(value);
  const baseline = useRef("");
  const nav = useRef<HTMLElement>(null);
  const focusIndex = useRef(0);
  const formId = useId();
  const edit = (index: number, add = false) => {
    baseline.current = JSON.stringify(value);
    focusIndex.current = index;
    setItems(add ? [...value, { display_text: "" }] : value);
    setOpen(true);
  };
  const move = (index: number, offset: number) => setItems(current => {
    const next = [...current];
    [next[index], next[index + offset]] = [next[index + offset], next[index]];
    return next;
  });
  return <>
    <nav ref={nav} className="ast-breadcrumb-editor" aria-label="页面路径">
      <ol>{value.map((item, index) => <li key={index}>
        {index > 0 ? <ChevronRight size={14} aria-hidden /> : null}
        {editable ? <button type="button" aria-label={`编辑路径项：${item.display_text}`} onClick={() => edit(index)}>{item.display_text}</button> : <span>{item.display_text}</span>}
      </li>)}</ol>
      {editable ? <Button type="button" variant="ghost" size="sm" disabled={value.length >= 32} onClick={() => edit(value.length, true)}><Plus aria-hidden />添加路径项</Button> : null}
    </nav>
    <Dialog open={open && editable} onOpenChange={setOpen}><DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-xl" onKeyDown={event => event.stopPropagation()} onOpenAutoFocus={event => {
      event.preventDefault(); document.getElementById(`${formId}-${focusIndex.current}`)?.focus();
    }} onCloseAutoFocus={event => {
      const button = nav.current?.querySelectorAll<HTMLButtonElement>("ol button")[Math.min(focusIndex.current, value.length - 1)];
      if (button) { event.preventDefault(); button.focus(); }
    }}><DialogHeader><DialogTitle>编辑页面路径</DialogTitle><DialogDescription className="sr-only">按顺序选择页面，也可以保留无链接的文字。</DialogDescription></DialogHeader>
      {items.map((item, index) => <div key={index} className="ast-path-item">
        <div className="flex min-w-0 items-center gap-2">
          <Label htmlFor={`${formId}-${index}`} className="shrink-0 text-muted-foreground">{index + 1}</Label>
          <Input id={`${formId}-${index}`} aria-label={`第 ${index + 1} 项名称`} placeholder="路径名称" maxLength={255} value={item.display_text} onChange={event => setItems(items.map((old, i) => i === index ? { ...old, display_text: event.target.value } : old))} />
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <ResourcePicker kind="page" value={item.target_page_id ?? ""} clearable onChange={choice => setItems(items.map((old, i) => i === index ? { display_text: choice.label || old.display_text, ...(choice.id ? { target_page_id: choice.id } : {}) } : old))} />
          <div className="flex gap-1">
            <Button type="button" size="icon-sm" variant="ghost" aria-label={`向前移动第 ${index + 1} 项`} disabled={index === 0} onClick={() => move(index, -1)}><ArrowUp aria-hidden /></Button>
            <Button type="button" size="icon-sm" variant="ghost" aria-label={`向后移动第 ${index + 1} 项`} disabled={index === items.length - 1} onClick={() => move(index, 1)}><ArrowDown aria-hidden /></Button>
            <Button type="button" size="icon-sm" variant="ghost" aria-label={`移除第 ${index + 1} 项`} disabled={items.length === 1} onClick={() => setItems(items.filter((_, i) => i !== index))}><Trash2 aria-hidden /></Button>
          </div>
        </div>
      </div>)}
      <Button type="button" variant="outline" disabled={items.length >= 32} onClick={() => setItems([...items, { display_text: "" }])}><Plus aria-hidden />添加路径项</Button>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => setOpen(false)}>取消</Button>
        <Button type="button" disabled={items.some(item => !item.display_text.trim())} onClick={() => {
          if (baseline.current !== JSON.stringify(value)) { toast.error("页面路径已更新，请重新打开后编辑。"); setOpen(false); return; }
          if (onChange(items.map(item => ({ ...item, display_text: item.display_text.trim() })))) setOpen(false);
        }}>保存页面路径</Button>
      </DialogFooter>
    </DialogContent></Dialog>
  </>;
}
