"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState, type FormEvent } from "react";

import { api, unwrap } from "@/lib/api/client";
import { errorMessage, fieldMessage } from "@/lib/api/errors";
import { useAuth } from "@/lib/auth/store";

type Mode = "login" | "register";

// Đăng nhập rồi lấy hồ sơ (/me) để có username hiển thị
async function signInWith(email: string, password: string) {
  const login = unwrap(await api.POST("/auth/login", { body: { email, password } })).data;
  // /me cần token: lưu tạm token trước, middleware tự gắn Authorization
  useAuth.setState({ token: login.access_token });
  const me = unwrap(await api.GET("/me")).data;
  useAuth.getState().signIn(login.access_token, { id: me.id, username: me.username });
}

export function AuthForm({ mode }: { mode: Mode }) {
  const router = useRouter();
  const next = useSearchParams().get("next") || "/";
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const email = String(form.get("email") ?? "");
    const password = String(form.get("password") ?? "");

    setBusy(true);
    setError(null);
    try {
      if (mode === "register") {
        unwrap(await api.POST("/auth/register", { body: { username: String(form.get("username") ?? ""), email, password } }));
      }
      await signInWith(email, password);
      router.replace(next.startsWith("/") ? next : "/");
    } catch (err) {
      useAuth.getState().signOut();
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  const title = mode === "login" ? "Đăng nhập" : "Tạo tài khoản";
  return (
    <div className="mx-auto mt-10 max-w-sm">
      <h1 className="mb-6 text-2xl font-bold">{title}</h1>
      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        {mode === "register" && (
          <Field label="Tên đăng nhập" name="username" autoComplete="username" error={fieldMessage(error, "username")} />
        )}
        <Field label="Email" name="email" type="email" autoComplete="email" error={fieldMessage(error, "email")} />
        <Field
          label="Mật khẩu"
          name="password"
          type="password"
          autoComplete={mode === "login" ? "current-password" : "new-password"}
          error={fieldMessage(error, "password")}
        />
        {error != null && (
          <p role="alert" className="text-sm text-red-400">
            {errorMessage(error)}
          </p>
        )}
        <button
          type="submit"
          disabled={busy}
          className="w-full rounded-lg bg-pink-600 py-2.5 font-medium text-white hover:bg-pink-500 disabled:opacity-50"
        >
          {busy ? "Đang xử lý…" : title}
        </button>
      </form>
      <p className="mt-4 text-sm text-neutral-400">
        {mode === "login" ? (
          <>
            Chưa có tài khoản? <Link href={`/register?next=${encodeURIComponent(next)}`} className="text-pink-400">Đăng ký</Link>
          </>
        ) : (
          <>
            Đã có tài khoản? <Link href={`/login?next=${encodeURIComponent(next)}`} className="text-pink-400">Đăng nhập</Link>
          </>
        )}
      </p>
    </div>
  );
}

function Field({ label, error, ...input }: { label: string; error?: string } & React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <label className="block">
      <span className="mb-1 block text-sm text-neutral-300">{label}</span>
      <input
        {...input}
        aria-invalid={error ? true : undefined}
        className={`w-full rounded-lg border bg-neutral-900 px-3 py-2 outline-none focus:border-pink-500 ${
          error ? "border-red-500" : "border-white/10"
        }`}
      />
      {error && <span className="mt-1 block text-xs text-red-400">{error}</span>}
    </label>
  );
}
