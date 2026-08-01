import { SourceWorkspace } from "@/components/sources/source-workspace";

export default async function SourceDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-10 lg:px-8 lg:py-12">
      <SourceWorkspace id={id} />
    </div>
  );
}
