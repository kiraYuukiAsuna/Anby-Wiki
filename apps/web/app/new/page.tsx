// /new：创建页面流程入口（M2-T03）。
// 服务端壳仅负责 Suspense 边界（useSearchParams 需要）；创建逻辑在客户端。
import { Suspense } from "react";

import { NewPageFlow } from "./new-page-flow";

export const dynamic = "force-dynamic";

export default function NewPage() {
  return (
    <Suspense>
      <NewPageFlow />
    </Suspense>
  );
}
