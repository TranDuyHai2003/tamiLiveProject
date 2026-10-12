"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";

import { useAuth } from "@/lib/auth/store";

export function Header() {
  const { user, hydrated, signOut } = useAuth();
  const router = useRouter();

  return (
    <header className="border-b border-white/10 bg-neutral-950">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-4">
        <Link href="/" className="text-lg font-bold text-pink-500">
          TamiLive
        </Link>
        <nav className="flex items-center gap-3 text-sm">
          {!hydrated ? null : user ? (
            <>
              <Link href="/live" className="rounded-full bg-pink-600 px-4 py-1.5 font-medium text-white hover:bg-pink-500">
                Lên sóng
              </Link>
              <span className="text-neutral-400" data-testid="current-user">
                {user.username}
              </span>
              <button
                className="text-neutral-400 hover:text-white"
                onClick={() => {
                  signOut();
                  router.push("/");
                }}
              >
                Đăng xuất
              </button>
            </>
          ) : (
            <>
              <Link href="/login" className="text-neutral-300 hover:text-white">
                Đăng nhập
              </Link>
              <Link href="/register" className="rounded-full bg-pink-600 px-4 py-1.5 font-medium text-white hover:bg-pink-500">
                Đăng ký
              </Link>
            </>
          )}
        </nav>
      </div>
    </header>
  );
}
