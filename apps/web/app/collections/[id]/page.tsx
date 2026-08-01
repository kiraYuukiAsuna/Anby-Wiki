import { CollectionWorkspace } from "@/components/collections/collection-workspace";

export default async function CollectionDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-10 lg:px-8 lg:py-12">
      <CollectionWorkspace id={id} />
    </div>
  );
}
