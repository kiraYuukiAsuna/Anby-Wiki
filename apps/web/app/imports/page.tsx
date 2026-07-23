import { ImportJobForm } from "@/components/imports/import-job-form";

export default function ImportsPage() {
  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-6 px-4 py-8">
      <header>
        <p className="text-xs font-medium tracking-widest text-muted-foreground uppercase">AI Import</p>
        <h1 className="mt-1 text-2xl font-bold tracking-tight">导入来源</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          来源会经过安全获取、文本解析、结构化抽取、实体消歧和 Claim 冲突检查，最终只生成待审核 Proposal。
        </p>
      </header>
      <ImportJobForm />
    </div>
  );
}
