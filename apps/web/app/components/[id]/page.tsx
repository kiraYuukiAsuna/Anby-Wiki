import { ComponentWorkspace } from "@/components/components/component-workspace";

export default async function ComponentPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-10 lg:px-8 lg:py-12">
      <ComponentWorkspace id={id} />
    </div>
  );
}
