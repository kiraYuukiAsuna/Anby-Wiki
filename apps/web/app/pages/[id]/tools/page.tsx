import { ArrowLeft, Wrench } from "lucide-react";
import Link from "next/link";
import { notFound } from "next/navigation";

import { BlockRedirectManager } from "@/components/page-tools/block-redirect-manager";
import { BacklinksPanel } from "@/components/page-tools/backlinks-panel";
import { PageEntityBindingManager } from "@/components/page-tools/page-entity-binding-manager";
import { PageRedirectManager } from "@/components/page-tools/page-redirect-manager";
import { Button } from "@/components/ui/button";
import { fetchPageById } from "@/lib/reading";

export const dynamic = "force-dynamic";

export default async function PageToolsPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const pageResult = await fetchPageById(id);
  if (pageResult.kind !== "ok") notFound();

  const { page, redirect } = pageResult.data;
  const managedPage = redirect
    ? { id: redirect.fromPageId, displayTitle: redirect.fromTitle }
    : page;

  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 flex flex-wrap items-end justify-between gap-6 border-b border-border/75 pb-8">
        <div>
          <span className="flex size-11 items-center justify-center rounded-2xl bg-cyan-100 text-cyan-800">
            <Wrench className="size-5" aria-hidden />
          </span>
          <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-cyan-800 uppercase">
            Page operations
          </p>
          <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
            页面工具
          </h1>
          <p className="mt-3 max-w-3xl text-sm leading-7 text-muted-foreground">
            管理「{managedPage.displayTitle}」的稳定地址与治理边界。正文内容仍通过版本化编辑器发布。
          </p>
        </div>
        <Button asChild variant="outline">
          <Link href={`/pages/${managedPage.id}`}>
            <ArrowLeft aria-hidden />
            返回阅读
          </Link>
        </Button>
      </header>
      <div className="space-y-6">
        <BacklinksPanel
          pageId={managedPage.id}
          pageTitle={managedPage.displayTitle}
        />
        <PageEntityBindingManager
          pageId={managedPage.id}
          pageTitle={managedPage.displayTitle}
          pageLanguage={page.language}
        />
        <PageRedirectManager
          pageId={managedPage.id}
          pageTitle={managedPage.displayTitle}
        />
        <BlockRedirectManager
          pageId={managedPage.id}
          pageTitle={managedPage.displayTitle}
        />
      </div>
    </div>
  );
}
