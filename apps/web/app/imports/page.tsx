import { ImportJobForm } from "@/components/imports/import-job-form";
import { ImportJobList } from "@/components/imports/import-job-list";

export default function ImportsPage() {
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-8 lg:px-8 lg:py-10">
      <header className="max-w-3xl">
        <p className="text-xs font-semibold tracking-[0.18em] text-primary uppercase">
          AI knowledge pipeline
        </p>
        <h1 className="mt-2 text-3xl font-semibold tracking-[-0.035em] sm:text-4xl">
          导入中心
        </h1>
        <p className="mt-3 text-sm leading-7 text-muted-foreground sm:text-base">
          把网页、API 快照、数据库导出、PDF、图片或文本变成可核验的实体、
          事实和引用。AI 只生成结构化提议，正式内容仍由审核与版本系统接管。
        </p>
      </header>

      <div className="mt-9 grid items-start gap-8 lg:grid-cols-[minmax(0,1fr)_22rem] xl:gap-12">
        <ImportJobList />
        <aside className="lg:sticky lg:top-24">
          <div className="mb-4">
            <p className="text-xs font-semibold tracking-[0.16em] text-primary uppercase">
              New import
            </p>
            <h2 className="mt-1 text-xl font-semibold">创建导入任务</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              提交后可以安全离开，任务会在队列中持续运行。
            </p>
          </div>
          <ImportJobForm />
        </aside>
      </div>
    </div>
  );
}
