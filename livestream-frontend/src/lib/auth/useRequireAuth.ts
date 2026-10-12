"use client";

import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";

import { useAuth } from "./store";

// Trang cần đăng nhập: chưa đăng nhập thì chuyển sang /login, đăng nhập xong quay lại trang cũ.
// Trả về token khi đã sẵn sàng, null khi còn đang đọc phiên hoặc đang chuyển trang.
export function useRequireAuth(): string | null {
  const { token, hydrated } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    if (hydrated && !token) router.replace(`/login?next=${encodeURIComponent(pathname)}`);
  }, [hydrated, token, router, pathname]);

  return hydrated ? token : null;
}
