"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";

import { useAuth } from "@/lib/auth/store";

import { useRoomChat } from "./useRoomChat";

const MAX_CHARS = 500; // khớp giới hạn của server

export function ChatPanel({ roomId, token }: { roomId: string; token: string }) {
  const { messages, status, send } = useRoomChat(roomId, token);
  const me = useAuth((s) => s.user);
  const [draft, setDraft] = useState("");
  const listRef = useRef<HTMLOListElement>(null);

  // Tự cuộn xuống tin mới nhất
  useEffect(() => {
    listRef.current?.scrollTo({ top: listRef.current.scrollHeight });
  }, [messages]);

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (send(draft)) setDraft("");
  }

  return (
    <aside className="flex h-[480px] flex-col rounded-xl border border-white/10 bg-neutral-900 lg:h-auto">
      <div className="flex items-center justify-between border-b border-white/10 px-4 py-2 text-sm">
        <span className="font-medium">Chat</span>
        <span className="text-xs text-neutral-500" data-testid="chat-status">
          {status === "open" ? "đã kết nối" : status === "connecting" ? "đang kết nối…" : "mất kết nối"}
        </span>
      </div>
      <ol ref={listRef} className="flex-1 space-y-1.5 overflow-y-auto px-4 py-3 text-sm" data-testid="chat-messages">
        {messages.map((m) => (
          <li key={m.id} data-testid="chat-message">
            <span className={m.user_id === me?.id ? "font-medium text-pink-400" : "font-medium text-sky-400"}>
              {m.username}
            </span>{" "}
            <span className="break-words text-neutral-200">{m.content}</span>
          </li>
        ))}
      </ol>
      <form onSubmit={onSubmit} className="flex gap-2 border-t border-white/10 p-3">
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          maxLength={MAX_CHARS}
          placeholder={status === "open" ? "Nhắn gì đó…" : "Đang kết nối chat…"}
          disabled={status !== "open"}
          data-testid="chat-input"
          className="min-w-0 flex-1 rounded-lg border border-white/10 bg-neutral-950 px-3 py-2 text-sm outline-none focus:border-pink-500 disabled:opacity-50"
        />
        <button
          type="submit"
          disabled={status !== "open" || !draft.trim()}
          className="rounded-lg bg-pink-600 px-3 text-sm font-medium text-white disabled:opacity-40"
        >
          Gửi
        </button>
      </form>
    </aside>
  );
}
