// 根级 404：未匹配任何路由（如 /wiki/a/b 这类含斜杠标题的多段路径）。
import Link from "next/link";

export default function NotFound() {
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-1 flex-col items-center justify-center gap-4 px-4 py-16 text-center">
      <h1 className="text-2xl font-semibold">404 · 页面不存在</h1>
      <p className="max-w-md text-sm text-muted-foreground">
        你访问的地址不存在。如果你要访问标题中含斜杠「/」的页面——这类标题暂不支持，
        将在后续版本提供。
      </p>
      <Link href="/" className="text-sm text-blue-600 hover:underline">
        返回首页
      </Link>
    </div>
  );
}
