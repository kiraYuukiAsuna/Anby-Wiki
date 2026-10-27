import { ArrowLeft, Wrench } from "lucide-react";
import Link from "next/link";
import { notFound } from "next/navigation";

import { BlockRedirectManager } from "@/components/page-tools/block-redirect-manager";
import { BacklinksPanel } from "@/components/page-tools/backlinks-panel";
import { PageEntityBindingManager } from "@/components/page-tools/page-entity-binding-manager";
import { PageRedirectManager } from "@/components/page-tools/page-redirect-manager";
import { PageShareManager } from "@/components/page-tools/page-share-manager";
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
      <header className="mb-8 flex flex-wrap items-center justify-between gap-4 border-b border-border/75 pb-6">
        <div className="flex items-center gap-3">
          <span className="flex size-11 items-center justify-center rounded-2xl bg-cyan-100 text-cyan-800">
            <Wrench className="size-5" aria-hidden />
          </span>
          <h1 className="text-3xl font-semibold">
            页面工具：{managedPage.displayTitle}
          </h1>
        </div>
        <Button asChild variant="outline">
          <Link href={`/pages/${managedPage.id}`}>
            <ArrowLeft aria-hidden />
            返回阅读
          </Link>
        </Button>
      </header>
      <div className="space-y-6">
        <PageShareManager
          pageId={managedPage.id}
          pageTitle={managedPage.displayTitle}
        />
        <BacklinksPanel pageId={managedPage.id} />
        <PageEntityBindingManager
          pageId={managedPage.id}
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
