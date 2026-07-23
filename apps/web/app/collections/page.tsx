import { CollectionList } from "@/components/collections/collection-list";
import { collectionsApi } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function CollectionsPage() {
  const initialPage = await collectionsApi().listCollections({ pageSize: 20 });

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-8">
      <header>
        <p className="text-xs font-medium tracking-widest text-muted-foreground uppercase">
          Collections
        </p>
        <h1 className="mt-1 text-2xl font-bold tracking-tight">合集</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          浏览人工维护或按 Entity 类型与已发布 Claim 规则物化的知识集合。
        </p>
      </header>
      <CollectionList initialPage={initialPage} />
    </div>
  );
}
