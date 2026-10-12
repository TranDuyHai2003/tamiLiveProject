"use client";

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

// NỢ KỸ THUẬT (trả ở bước 8 — docs/ROADMAP.md):
// access token đang nằm trong localStorage. Mọi script chạy trên trang (kể cả script bị chèn qua XSS)
// đọc được localStorage. Bước 8 chuyển sang: access token chỉ nằm trong bộ nhớ + refresh token trong
// cookie HttpOnly mà JavaScript không đọc được.

export type SessionUser = { id: string; username: string };

type AuthState = {
  token: string | null;
  user: SessionUser | null;
  // false cho tới khi đọc xong localStorage. Trước đó chưa biết đã đăng nhập hay chưa,
  // nên UI phụ thuộc đăng nhập phải chờ (tránh nháy "Đăng nhập" rồi mới hiện tên).
  hydrated: boolean;
  signIn: (token: string, user: SessionUser) => void;
  signOut: () => void;
};

export const useAuth = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      hydrated: false,
      signIn: (token, user) => set({ token, user }),
      signOut: () => set({ token: null, user: null }),
    }),
    {
      name: "tamilive-auth",
      storage: createJSONStorage(() => localStorage),
      partialize: (s) => ({ token: s.token, user: s.user }),
      // Đọc localStorage ở client sau khi trang hiển thị (server không có localStorage)
      skipHydration: true,
      onRehydrateStorage: () => () => useAuth.setState({ hydrated: true }),
    },
  ),
);
