"use client";

import { useEffect } from "react";

import { useAuth } from "@/lib/auth/store";

// Đọc phiên đăng nhập từ localStorage sau khi trang đã hiển thị ở client.
// Làm trong effect (không làm lúc render) để HTML do server render và lần render đầu ở client giống nhau.
export function AuthHydrator() {
  useEffect(() => {
    void useAuth.persist.rehydrate();
  }, []);
  return null;
}
