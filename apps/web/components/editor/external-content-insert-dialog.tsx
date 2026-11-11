"use client";

import { useEffect, useRef, useState } from "react";
import { LoaderCircle, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { BlockView } from "@/components/ast";
import { bookmarkBlockSchema, embedBlockSchema, linkPreviewBlockSchema, type Block } from "@/lib/ast/schema";
import { sourcesApi } from "@/lib/api";
import { detectEmbedProvider } from "@/lib/embed";
import { safeHttpUrl } from "@/lib/http-url";
import { newBlockId } from "@/lib/editor/ids";
import type { LinkPreviewMetadata } from "../../../../contracts/generated/typescript";

type ExternalType = "bookmark" | "link_preview" | "embed";
export type ExternalBlock = Extract<Block, { type: ExternalType }>;
const names = { bookmark: "书签", link_preview: "链接预览", embed: "嵌入内容" };

export function ExternalContentInsertDialog({ open, onOpenChange, resolvePreview, initialBlock, onInsert }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  resolvePreview?: (url: string) => Promise<LinkPreviewMetadata>;
  initialBlock?: ExternalBlock;
  onInsert: (block: ExternalBlock) => void | boolean;
}) {
  const [type, setType] = useState<ExternalType>(initialBlock?.type ?? "bookmark");
  const [url, setURL] = useState(initialBlock?.url ?? "");
  const [title, setTitle] = useState(initialBlock?.title ?? "");
  const [description, setDescription] = useState(initialBlock && "description" in initialBlock ? initialBlock.description ?? "" : "");
  const [siteName, setSiteName] = useState(initialBlock && "site_name" in initialBlock ? initialBlock.site_name ?? "" : "");
  const [caption, setCaption] = useState(initialBlock && "caption" in initialBlock ? initialBlock.caption ?? "" : "");
  const [resolving, setResolving] = useState(false);
  const request = useRef(0);
  useEffect(() => () => { request.current++; }, []);
  const changeOpen = (next: boolean) => { if (!next) request.current++; onOpenChange(next); };
  const resolve = async () => {
    const address = safeHttpUrl(url);
    if (!address) { toast.error("请输入完整的 HTTP(S) 链接。"); return null; }
    const pending = ++request.current;
    setResolving(true);
    try {
      const preview = resolvePreview ? await resolvePreview(address) : await sourcesApi().resolveLinkPreview({ resolveLinkPreviewRequest: { url: address } });
      if (pending !== request.current) return null;
      setURL(preview.url); setTitle(preview.title); setDescription(preview.description ?? ""); setSiteName(preview.siteName ?? "");
      return preview;
    } catch {
      if (pending === request.current) toast.error("无法获取网页信息", { description: "可重试，或手动填写标题后保存。" });
      return null;
    } finally { if (pending === request.current) setResolving(false); }
  };
  const submit = async () => {
    const id = initialBlock?.id ?? newBlockId();
    if (type === "embed") {
      const provider = detectEmbedProvider(url);
      if (!provider) { toast.error("交互式 Embed 只支持 YouTube、Vimeo 与 Figma"); return; }
      const parsed = embedBlockSchema.safeParse({ id, type, url: url.trim(), ...(title.trim() ? { title: title.trim() } : {}), provider, ...(caption.trim() ? { caption: caption.trim() } : {}) });
      if (!parsed.success) { toast.error("嵌入内容配置无效", { description: parsed.error.issues[0]?.message }); return; }
      if (onInsert(parsed.data) !== false) changeOpen(false);
      return;
    }
    let metadata = { url: url.trim(), title: title.trim(), description: description.trim(), siteName: siteName.trim() };
    if (!metadata.title) {
      const preview = await resolve();
      if (!preview) return;
      metadata = { url: preview.url, title: preview.title, description: preview.description ?? "", siteName: preview.siteName ?? "" };
    }
    const common = { id, type, url: metadata.url, title: metadata.title, ...(metadata.description ? { description: metadata.description } : {}), ...(metadata.siteName ? { site_name: metadata.siteName } : {}) };
    const parsed = type === "bookmark" ? bookmarkBlockSchema.safeParse({ ...common, ...(caption.trim() ? { caption: caption.trim() } : {}) }) : linkPreviewBlockSchema.safeParse(common);
    if (!parsed.success) { toast.error("链接预览配置无效", { description: parsed.error.issues[0]?.message }); return; }
    if (onInsert(parsed.data as ExternalBlock) !== false) changeOpen(false);
  };
  return <Dialog open={open} onOpenChange={changeOpen}>
    <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl" onKeyDown={event => event.stopPropagation()}>
      <DialogHeader><DialogTitle>{initialBlock ? `编辑${names[type]}` : "插入外部内容"}</DialogTitle><DialogDescription>标题和摘要会保存在页面中，可随时重新获取或手动修改。</DialogDescription></DialogHeader>
      <fieldset disabled={resolving} className="grid min-w-0 gap-4">
        {!initialBlock && <Label className="grid gap-2">类型<select value={type} onChange={event => setType(event.target.value as ExternalType)} className="h-9 w-full rounded-lg border border-input bg-background px-2.5 text-sm">{Object.entries(names).map(([value, name]) => <option key={value} value={value}>{name}</option>)}</select></Label>}
        <Label className="grid gap-2">链接地址<Input type="url" placeholder="https://…" value={url} onChange={event => setURL(event.target.value)} /></Label>
        {type !== "embed" && <Button type="button" variant="outline" className="justify-self-start" disabled={!url.trim()} onClick={() => void resolve()}>{resolving ? <LoaderCircle className="animate-spin" aria-hidden /> : <RefreshCw aria-hidden />}获取网页信息</Button>}
        <Label className="grid gap-2">标题<Input maxLength={type === "embed" ? undefined : 500} value={title} onChange={event => setTitle(event.target.value)} /></Label>
        {type !== "embed" && <>
          <Label className="grid gap-2">站点名<Input maxLength={255} value={siteName} onChange={event => setSiteName(event.target.value)} /></Label>
          <Label className="grid gap-2">摘要<Textarea aria-label="摘要" maxLength={1000} value={description} onChange={event => setDescription(event.target.value)} /></Label>
        </>}
        {type !== "link_preview" && <Label className="grid gap-2">说明（可选）<Input maxLength={1000} value={caption} onChange={event => setCaption(event.target.value)} /></Label>}
      </fieldset>
      {type !== "embed" && <div aria-label="链接卡片预览" className="min-w-0" onClickCapture={event => { if ((event.target as Element).closest("a")) { event.preventDefault(); event.stopPropagation(); } }}>
        {safeHttpUrl(url) ? <BlockView block={{ id: initialBlock?.id ?? "link-preview", type, url: url.trim(), title: title || "链接标题", description, site_name: siteName, ...(type === "bookmark" ? { caption } : {}) }} /> : <p className="text-sm text-muted-foreground">填写链接后显示卡片预览。</p>}
      </div>}
      <DialogFooter><Button type="button" variant="outline" onClick={() => changeOpen(false)}>取消</Button><Button type="button" disabled={resolving} onClick={() => void submit()}>{initialBlock ? "保存链接" : "插入"}</Button></DialogFooter>
    </DialogContent>
  </Dialog>;
}
