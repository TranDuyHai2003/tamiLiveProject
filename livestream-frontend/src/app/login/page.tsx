import { Suspense } from "react";

import { AuthForm } from "@/components/AuthForm";

// AuthForm đọc ?next= bằng useSearchParams: với Cache Components, phần đọc URL phải nằm trong <Suspense>.
export default function LoginPage() {
  return (
    <Suspense>
      <AuthForm mode="login" />
    </Suspense>
  );
}
