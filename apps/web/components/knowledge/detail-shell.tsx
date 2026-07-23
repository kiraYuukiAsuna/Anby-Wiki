import type { ReactNode } from "react";

export function DetailShell({
  eyebrow,
  title,
  status,
  children,
}: {
  eyebrow: string;
  title: string;
  status: string;
  children: ReactNode;
}) {
  return (
    <main className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8">
      <header className="border-b border-border pb-5">
        <div className="mb-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <span className="uppercase tracking-[0.18em]">{eyebrow}</span>
          <span className="rounded-full border border-border px-2 py-0.5">{status}</span>
          <span className="rounded-full border border-border px-2 py-0.5">只读</span>
        </div>
        <h1 className="text-3xl font-bold tracking-tight">{title}</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          当前详情页匿名可读；所有权威写入仍须经过领域服务与授权工作流。
        </p>
      </header>
      {children}
    </main>
  );
}

export function DetailSection({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="rounded-xl border border-border p-5">
      <h2 className="mb-4 text-lg font-semibold">{title}</h2>
      {children}
    </section>
  );
}

export function DetailRows({
  rows,
}: {
  rows: Array<{ label: string; value: ReactNode }>;
}) {
  return (
    <dl className="grid gap-3 text-sm sm:grid-cols-[10rem_1fr]">
      {rows.map((row) => (
        <div className="contents" key={row.label}>
          <dt className="text-muted-foreground">{row.label}</dt>
          <dd className="min-w-0 break-words font-mono text-xs sm:text-sm">
            {row.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}
