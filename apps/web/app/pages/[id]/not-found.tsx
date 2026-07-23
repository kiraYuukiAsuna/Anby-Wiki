// /pages/[id] 的 404：ID 不存在。
import Link from "next/link";

export default function PageByIdNotFound() {
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-1 flex-col items-center justify-center gap-4 px-4 py-16 text-center">
      <h1 className="text-2xl font-semibold">页面不存在</h1>
      <p className="max-w-md text-sm text-muted-foreground">
        没有找到这个页面 ID 对应的页面，它可能已被移动或删除。
      </p>
      <Link href="/" className="text-sm text-blue-600 hover:underline">
        返回首页
      </Link>
    </div>
  );
}
