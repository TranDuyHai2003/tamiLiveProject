"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { ApiError, api, unwrap } from "@/lib/api/client";
import { errorMessage, fieldMessage } from "@/lib/api/errors";
import { useAuth } from "@/lib/auth/store";
import { useRequireAuth } from "@/lib/auth/useRequireAuth";

// Lên sóng: tạo phòng rồi chuyển sang trang phòng. Trang phòng dùng chung cho host và viewer;
// server trả về vai trò khi vào phòng và quyết định ai được phát hình.
export default function GoLivePage() {
  const token = useRequireAuth();
  const router = useRouter();
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setError(null);
    try {
      const room = unwrap(
        await api.POST("/rooms", {
          body: { title: String(form.get("title") ?? ""), category: String(form.get("category") ?? "") || undefined },
        }),
      ).data;
      router.push(`/rooms/${room.id}`);
    } catch (err) {
      if (err instanceof ApiError && err.code === "ALREADY_LIVE") {
        // Đang có phòng live (VD lỡ tải lại trang): tìm phòng của mình và vào lại
        const own = await findOwnLiveRoom();
        if (own) return router.push(`/rooms/${own}`);
      }
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  if (!token) return null;

  return (
    <div className="mx-auto mt-10 max-w-md">
      <h1 className="mb-6 text-2xl font-bold">Lên sóng</h1>
      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        <label className="block">
          <span className="mb-1 block text-sm text-neutral-300">Tiêu đề</span>
          <input
            name="title"
            required
            maxLength={255}
            placeholder="Hôm nay mình hát…"
            className="w-full rounded-lg border border-white/10 bg-neutral-900 px-3 py-2 outline-none focus:border-pink-500"
          />
          {fieldMessage(error, "title") && <span className="mt-1 block text-xs text-red-400">{fieldMessage(error, "title")}</span>}
        </label>
        <label className="block">
          <span className="mb-1 block text-sm text-neutral-300">Chủ đề (không bắt buộc)</span>
          <input
            name="category"
            placeholder="Âm nhạc"
            className="w-full rounded-lg border border-white/10 bg-neutral-900 px-3 py-2 outline-none focus:border-pink-500"
          />
        </label>
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
          {busy ? "Đang tạo phòng…" : "Bắt đầu phát"}
        </button>
        <p className="text-xs text-neutral-500">Trình duyệt sẽ hỏi quyền dùng camera và micro.</p>
      </form>
    </div>
  );
}

async function findOwnLiveRoom(): Promise<string | null> {
  const me = useAuth.getState().user;
  if (!me) return null;
  const page = unwrap(await api.GET("/rooms", { params: { query: { limit: 100 } } }));
  return page.data.find((r) => r.host_id === me.id)?.id ?? null;
}
