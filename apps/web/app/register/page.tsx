import { Suspense } from "react";

import { RegisterForm } from "./register-form";

export const dynamic = "force-dynamic";

export default function RegisterPage() {
  return (
    <Suspense>
      <RegisterForm />
    </Suspense>
  );
}
