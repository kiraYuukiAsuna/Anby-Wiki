import Link from "next/link";
import { notFound } from "next/navigation";
import { ResponseError } from "../../../../../contracts/generated/typescript";

import { CollectionMembers } from "@/components/collections/collection-members";
import { Button } from "@/components/ui/button";
import { collectionsApi } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function CollectionDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  let result;
  try {
    result = await Promise.all([
      collectionsApi().getCollection({ id }),
      collectionsApi().listCollectionMembers({ id, pageSize: 20 }),
    ]);
  } catch (error) {
    if (error instanceof ResponseError && error.response.status === 404) {
      notFound();
    }
    throw error;
  }
  const [collection, members] = result;
  const rule =
    collection.collectionType === "manual" || !collection.query
      ? "人工维护"
      : collection.query.kind === "entity_type"
        ? `EntityType = ${collection.query.entityType ?? "未知"}`
        : `存在 published Claim：${collection.query.property ?? "未知"}`;

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-8">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-xs font-medium tracking-widest text-muted-foreground uppercase">
            {collection.collectionType} collection
          </p>
          <h1 className="mt-1 text-2xl font-bold tracking-tight">
            {collection.title}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">{rule}</p>
        </div>
        <Button variant="outline" size="sm" asChild>
          <Link href="/collections">返回合集</Link>
        </Button>
      </header>

      <section className="rounded-lg border border-border p-4">
        <h2 className="font-semibold">定义</h2>
        <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
          <div>
            <dt className="text-muted-foreground">Collection ID</dt>
            <dd className="mt-1 break-all font-mono">{collection.id}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">描述页面</dt>
            <dd className="mt-1">
              {collection.descriptionPageId ? (
                <Link
                  className="text-blue-600 hover:underline"
                  href={`/pages/${collection.descriptionPageId}`}
                >
                  {collection.descriptionPageId}
                </Link>
              ) : (
                "未设置"
              )}
            </dd>
          </div>
        </dl>
      </section>

      <section>
        <h2 className="mb-3 text-lg font-semibold">成员</h2>
        <CollectionMembers collectionId={collection.id} initialPage={members} />
      </section>
    </div>
  );
}
