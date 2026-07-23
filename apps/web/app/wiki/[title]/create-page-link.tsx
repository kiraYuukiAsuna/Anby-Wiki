// 「创建此页面」链接（M2-T03）：/wiki/[title] 404 页使用。
// not-found 组件拿不到路由参数，在客户端从地址栏恢复标题，
// 拼出 /new?title=...&namespace=main；恢复失败退化为 /new。
"use client";

import { useSyncExternalStore } from "react";
import Link from "next/link";

import { Button } from "@/components/ui/button";

const FALLBACK_HREF = "/new";

function computeHref(pathname: string): string {
  const raw = pathname.replace(/^\/wiki\//, "");
  try {
    const title = decodeURIComponent(raw);
    // 含斜杠的标题 v1 不支持（与 /wiki/[title] 的拦截一致），不给预填。
    if (title && !title.includes("/")) {
      return `/new?title=${encodeURIComponent(title)}&namespace=main`;
    }
  } catch {
    // 非法编码：保持 /new 兜底。
  }
  return FALLBACK_HREF;
}

export function CreatePageLink() {
  // 地址栏是浏览器外部状态：useSyncExternalStore（服务端快照用兜底链接，
  // 客户端水合后替换为预填链接）。
  const href = useSyncExternalStore(
    () => () => {},
    () => computeHref(window.location.pathname),
    () => FALLBACK_HREF,
  );

  return (
    <Button size="sm" asChild>
      <Link href={href}>创建此页面</Link>
    </Button>
  );
}
