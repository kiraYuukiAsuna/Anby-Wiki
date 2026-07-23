import { ImportJobProgress } from "@/components/imports/import-job-progress";

export default async function ImportJobPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8">
      <header>
        <p className="text-xs font-medium tracking-widest text-muted-foreground uppercase">AI Import</p>
        <h1 className="mt-1 text-2xl font-bold tracking-tight">导入任务进度</h1>
      </header>
      <ImportJobProgress id={id} />
    </div>
  );
}
