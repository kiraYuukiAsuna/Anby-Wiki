"use client";

import Link from "next/link";
import { useState } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import type { PageTranslations } from "../../../contracts/generated/typescript";
import { Button } from "@/components/ui/button";
import { readingApi } from "@/lib/api";

export function PageLanguageSwitcher({ pageId, language, canTranslate, initial }: {
  pageId: string; language: string; canTranslate: boolean; initial: PageTranslations;
}) {
  const [submitting, setSubmitting] = useState(false);
  const { data = initial, mutate } = useSWR(["page-translations", pageId],
    () => readingApi().listPageTranslations({ id: pageId }), {
      fallbackData: initial,
      refreshInterval: (value) => value?.status === "pending" || value?.status === "failed" ? 3000 : 0,
    });
  const supported = /^(zh|en)(-|$)/i.test(language);
  const target = language.toLowerCase().startsWith("en") ? "中文" : "English";
  async function translate() {
    setSubmitting(true);
    try {
      await readingApi().requestPageTranslation({ id: pageId });
      await mutate();
      toast.success("已加入翻译队列");
    } catch { toast.error("暂时无法开始翻译，请稍后重试"); }
    finally { setSubmitting(false); }
  }
  return <div className="mb-5 flex flex-wrap items-center gap-2" aria-label="页面语言">
    {data.items.map((item) => <Button key={item.pageId} variant={item.pageId === pageId ? "secondary" : "ghost"} size="sm" asChild>
      <Link href={`/pages/${item.pageId}?language=${encodeURIComponent(item.language)}`} hrefLang={item.language} aria-current={item.pageId === pageId ? "page" : undefined}>
        {item.language.toLowerCase().startsWith("zh") ? "中文" : item.language.toLowerCase().startsWith("en") ? "English" : item.language}
      </Link>
    </Button>)}
    {canTranslate && supported && data.items.length < 2 && data.status !== "done" ? <Button size="sm" variant="outline" disabled={submitting || data.status === "pending"} onClick={translate}>
      {submitting || data.status === "pending" ? "翻译中…" : data.status === "failed" ? "重试翻译" : `翻译为${target}`}
    </Button> : null}
    {data.status === "failed" ? <span role="status" className="text-xs text-muted-foreground">翻译暂未完成，原文不受影响</span> : null}
  </div>;
}
