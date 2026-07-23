import { BookOpen } from "lucide-react";

export default function Home() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-6 px-6 text-center">
      <BookOpen className="size-10 text-muted-foreground" aria-hidden />
      <div className="flex flex-col gap-2">
        <h1 className="text-3xl font-semibold tracking-tight">Anby Wiki</h1>
        <p className="max-w-md text-muted-foreground">
          人工与 AI 共同维护的百科。通过{" "}
          <code className="rounded bg-muted px-1 py-0.5 font-mono text-sm">
            /wiki/页面标题
          </code>{" "}
          阅读页面。
        </p>
      </div>
    </div>
  );
}
