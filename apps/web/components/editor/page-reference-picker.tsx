// 页面引用选择器（M2-T04）：cmdk 可搜索选择器，输入即搜索（SWR + 200ms 防抖
// 调 GET /api/v1/pages/search）。选项为既有页面（选中 → 已解析引用，display_text
// 默认页面标题、插入前可改）；列表底部常驻「创建未解析引用：{输入}」选项。
// 组件只产出结构化回调（目标 ID / 显示文本 / 未解析标题），不直接触碰编辑器。
"use client";

import { useEffect, useState } from "react";
import { Command } from "cmdk";
import { FilePlus2, FileText } from "lucide-react";
import useSWR from "swr";

import type { PageSearchHit } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { searchApi } from "@/lib/api";

export interface PageReferencePickerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** 选中既有页面并确认显示文本后回调（已解析引用）。 */
  onInsertResolved: (targetPageId: string, displayText: string) => void;
  /** 选择「创建未解析引用」后回调（rawTitle 为规范化前用户输入）。 */
  onInsertUnresolved: (rawTitle: string) => void;
}

const SEARCH_DEBOUNCE_MS = 200;
const SEARCH_LIMIT = 10;

/** 输入防抖：停止输入 SEARCH_DEBOUNCE_MS 后才真正发请求。 */
function useDebouncedValue(value: string, delayMs: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(timer);
  }, [value, delayMs]);
  return debounced;
}

export function PageReferencePicker({
  open,
  onOpenChange,
  onInsertResolved,
  onInsertUnresolved,
}: PageReferencePickerProps) {
  const [query, setQuery] = useState("");
  // 选中既有页面后进入第二步：确认/修改显示文本（与 target_page_id 分离）。
  const [selected, setSelected] = useState<PageSearchHit | null>(null);
  const [displayText, setDisplayText] = useState("");

  // 关闭/重开时重置内部状态（渲染期调整，避免 effect 级联渲染）。
  const [prevOpen, setPrevOpen] = useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    setQuery("");
    setSelected(null);
    setDisplayText("");
  }

  const debouncedQuery = useDebouncedValue(query, SEARCH_DEBOUNCE_MS);
  const trimmedQuery = query.trim();

  const { data, isLoading } = useSWR(
    open && debouncedQuery.trim() ? ["pages-search", debouncedQuery] : null,
    ([, q]) =>
      searchApi().searchPages({
        q,
        namespace: "main",
        fields: ["title", "alias"],
        limit: SEARCH_LIMIT,
      }),
    { keepPreviousData: true },
  );
  const hits = data?.items ?? [];

  const pickHit = (hit: PageSearchHit) => {
    setSelected(hit);
    setDisplayText(hit.displayTitle);
  };

  const confirmResolved = () => {
    if (!selected) return;
    onInsertResolved(selected.id, displayText.trim() || selected.displayTitle);
    onOpenChange(false);
  };

  const createUnresolved = () => {
    if (!trimmedQuery) return;
    onInsertUnresolved(trimmedQuery);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>插入页面引用</DialogTitle>
          <DialogDescription>
            搜索既有页面，或创建指向尚未存在页面的未解析引用。
          </DialogDescription>
        </DialogHeader>
        {selected ? (
          <div className="flex flex-col gap-4" data-picker-display-step>
            <div className="flex flex-col gap-2">
              <Label htmlFor="page-ref-display-text">显示文本</Label>
              <Input
                id="page-ref-display-text"
                value={displayText}
                onChange={(event) => setDisplayText(event.target.value)}
                placeholder={selected.displayTitle}
                autoFocus
              />
              <p className="text-xs text-muted-foreground">
                引用目标：{selected.displayTitle}
                {selected.matchedOn === "alias" ? "（经别名命中）" : ""}
                ；显示文本与目标分离，只影响本文档内的展示。
              </p>
            </div>
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setSelected(null)}
              >
                返回
              </Button>
              <Button size="sm" onClick={confirmResolved}>
                插入引用
              </Button>
            </div>
          </div>
        ) : (
          <Command shouldFilter={false} label="搜索页面">
            <Command.Input
              value={query}
              onValueChange={setQuery}
              placeholder="输入页面标题搜索…"
              className="border-input placeholder:text-muted-foreground flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]"
            />
            <Command.List className="mt-2 max-h-64 overflow-y-auto">
              {isLoading && hits.length === 0 ? (
                <Command.Loading>
                  <div className="px-3 py-2 text-sm text-muted-foreground">
                    搜索中…
                  </div>
                </Command.Loading>
              ) : null}
              {!isLoading && hits.length === 0 && trimmedQuery ? (
                <div className="px-3 py-2 text-sm text-muted-foreground">
                  没有匹配的既有页面
                </div>
              ) : null}
              {hits.map((hit) => (
                <Command.Item
                  key={hit.id}
                  value={hit.id}
                  onSelect={() => pickHit(hit)}
                  className="data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground flex cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-sm"
                >
                  <FileText className="size-4 shrink-0 text-muted-foreground" />
                  <span className="truncate">{hit.displayTitle}</span>
                  {hit.matchedOn === "alias" ? (
                    <span className="ml-auto text-xs text-muted-foreground">
                      别名命中
                    </span>
                  ) : null}
                </Command.Item>
              ))}
              {trimmedQuery ? (
                <Command.Item
                  value={`__create__:${trimmedQuery}`}
                  onSelect={createUnresolved}
                  className="data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground flex cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-sm"
                >
                  <FilePlus2 className="size-4 shrink-0 text-muted-foreground" />
                  <span className="truncate">
                    创建未解析引用：{trimmedQuery}
                  </span>
                </Command.Item>
              ) : null}
            </Command.List>
          </Command>
        )}
      </DialogContent>
    </Dialog>
  );
}
