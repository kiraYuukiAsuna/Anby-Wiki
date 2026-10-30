import { ImportJobForm } from "@/components/imports/import-job-form";
import { ImportJobList } from "@/components/imports/import-job-list";

export default function ImportsPage() {
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-8 lg:px-8 lg:py-10">
      <header className="border-b border-border/75 pb-6">
        <h1 className="text-3xl font-semibold">导入中心</h1>
      </header>

      <div className="mt-9 grid grid-cols-1 items-start gap-8 lg:grid-cols-[minmax(0,1fr)_22rem] xl:gap-12">
        <ImportJobList />
        <aside className="lg:sticky lg:top-24">
          <div className="mb-4">
            <h2 className="text-xl font-semibold">创建导入任务</h2>
          </div>
          <ImportJobForm />
        </aside>
      </div>
    </div>
  );
}
