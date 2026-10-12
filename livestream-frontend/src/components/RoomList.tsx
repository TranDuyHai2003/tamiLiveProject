"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import { api, unwrap, type Schemas } from "@/lib/api/client";
import { errorMessage } from "@/lib/api/errors";

type Room = Schemas["RoomSummary"];

const REFRESH_MS = 10_000;

export function RoomList() {
  const [rooms, setRooms] = useState<Room[] | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);

  // Trang đầu, tự làm mới định kỳ để thấy phòng mới lên sóng / vừa kết thúc
  const loadFirstPage = useCallback(async () => {
    try {
      const page = unwrap(await api.GET("/rooms", { params: { query: { limit: 20 } } }));
      setRooms(page.data);
      setNextCursor(page.next_cursor);
      setError(null);
    } catch (err) {
      setError(err);
    }
  }, []);

  useEffect(() => {
    // Gọi trong timeout để setState không chạy đồng bộ trong thân effect
    const first = setTimeout(loadFirstPage, 0);
    const timer = setInterval(loadFirstPage, REFRESH_MS);
    return () => {
      clearTimeout(first);
      clearInterval(timer);
    };
  }, [loadFirstPage]);

  async function loadMore() {
    if (!nextCursor) return;
    try {
      const page = unwrap(await api.GET("/rooms", { params: { query: { limit: 20, cursor: nextCursor } } }));
      setRooms((prev) => [...(prev ?? []), ...page.data]);
      setNextCursor(page.next_cursor);
    } catch (err) {
      setError(err);
    }
  }

  if (error && !rooms) return <p className="text-red-400">{errorMessage(error)}</p>;
  if (!rooms) return <p className="text-neutral-500">Đang tải…</p>;
  if (rooms.length === 0) return <p className="text-neutral-500">Chưa có ai đang live. Bấm “Lên sóng” để là người đầu tiên!</p>;

  return (
    <div>
      <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3" data-testid="room-list">
        {rooms.map((room) => (
          <li key={room.id}>
            <Link
              href={`/rooms/${room.id}`}
              data-testid="room-card"
              data-room-id={room.id}
              className="block rounded-xl border border-white/10 bg-neutral-900 p-4 hover:border-pink-500"
            >
              <div className="mb-3 flex aspect-video items-center justify-center rounded-lg bg-neutral-800 text-neutral-500">
                <span className="rounded bg-red-600 px-2 py-0.5 text-xs font-bold text-white">LIVE</span>
              </div>
              <p className="truncate font-medium">{room.title}</p>
              <p className="text-sm text-neutral-400">@{room.host_username}</p>
            </Link>
          </li>
        ))}
      </ul>
      {nextCursor && (
        <button onClick={loadMore} className="mt-6 rounded-lg border border-white/10 px-4 py-2 text-sm hover:border-pink-500">
          Xem thêm
        </button>
      )}
    </div>
  );
}
