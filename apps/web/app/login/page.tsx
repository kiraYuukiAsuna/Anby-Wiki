// /login：本地账号登录页。服务端壳只提供 useSearchParams 所需的 Suspense。
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
