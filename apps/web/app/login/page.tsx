// /login：早期阶段引导令牌登录页。
// 服务端壳仅负责 Suspense 边界（useSearchParams 需要）；登录逻辑在客户端。
import { Suspense } from "react";

import { LoginForm } from "./login-form";

export const dynamic = "force-dynamic";

export default function LoginPage() {
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  );
}
