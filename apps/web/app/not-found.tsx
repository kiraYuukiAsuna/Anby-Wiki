// 根级 404：未匹配任何应用路由。
import Link from "next/link";

export default function NotFound() {
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-1 flex-col items-center justify-center gap-4 px-4 py-16 text-center">
      <h1 className="text-2xl font-semibold">404 · 页面不存在</h1>
      <p className="max-w-md text-sm text-muted-foreground">
        你访问的地址不存在，可能已被移动、删除，或链接本身不完整。
      </p>
      <Link href="/" className="text-sm text-blue-600 hover:underline">
        返回首页
      </Link>
    </div>
  );
}
