import { BookOpenText } from "lucide-react";

import { cn } from "@/lib/utils";

export function AuthShell({
  children,
  footer,
  title,
  wide = false,
}: {
  children: React.ReactNode;
  footer: React.ReactNode;
  title: string;
  wide?: boolean;
}) {
  return (
    <main className="relative flex min-h-dvh items-center justify-center overflow-hidden bg-background px-5 py-10">
      <div className="wiki-grid pointer-events-none absolute inset-0 opacity-55" />

      <div
        className={cn(
          "relative w-full",
          wide ? "max-w-xl" : "max-w-md",
        )}
      >
        <div className="mb-8 flex items-center gap-3">
          <span className="flex size-10 items-center justify-center rounded-[8px] bg-primary text-primary-foreground shadow-sm">
            <BookOpenText className="size-5" aria-hidden />
          </span>
          <p className="text-sm font-semibold">Anby Wiki</p>
        </div>

        <section className="rounded-[8px] border bg-card/95 p-6 shadow-[0_24px_80px_-48px_rgba(15,23,42,0.55)] backdrop-blur-sm sm:p-8">
          <h1 className="text-2xl font-semibold">{title}</h1>

          <div className="mt-6">{children}</div>

          <div className="mt-7 border-t pt-6 text-center text-sm text-muted-foreground">
            {footer}
          </div>
        </section>
      </div>
    </main>
  );
}
