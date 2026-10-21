import { PublicDatasetFormView } from "@/components/datasets/public-dataset-form";

export default async function PublicDatasetFormPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <div className="mx-auto w-full max-w-5xl px-5 py-10 lg:px-8 lg:py-12">
      <PublicDatasetFormView id={id} />
    </div>
  );
}
