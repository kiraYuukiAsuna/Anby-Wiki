import { DatasetWorkspace } from "@/components/datasets/dataset-workspace";

export default async function DatasetPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <DatasetWorkspace id={id} />
    </div>
  );
}
