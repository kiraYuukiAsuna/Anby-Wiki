import { DatasetViewPage } from "@/components/datasets/dataset-view";

export default async function SavedDatasetViewPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <DatasetViewPage id={id} />
    </div>
  );
}
