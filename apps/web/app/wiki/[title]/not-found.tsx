// /wiki/[title] 的 404：页面不存在时的自定义说明，含「创建此页面」入口
// （M2-T03：链接到 /new 创建流程，标题由客户端从地址栏恢复预填）。
import { CreatePageLink } from "./create-page-link";

export default function WikiTitleNotFound() {
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-1 flex-col items-center justify-center gap-4 px-4 py-16 text-center">
      <h1 className="text-2xl font-semibold">页面不存在</h1>
      <p className="max-w-md text-sm text-muted-foreground">
        你访问的页面尚未创建，或标题中含有暂不支持的特殊字符（如斜杠
        「/」——含斜杠的标题将在后续版本支持）。
      </p>
      <CreatePageLink />
    </div>
  );
}
