"use client";

import { LiveKitRoom, RoomAudioRenderer } from "@livekit/components-react";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { ChatPanel } from "@/components/chat/ChatPanel";
import { ApiError, api, unwrap, type Schemas } from "@/lib/api/client";
import { errorMessage } from "@/lib/api/errors";
import { useRequireAuth } from "@/lib/auth/useRequireAuth";

import { Stage } from "./Stage";

type Joined = Schemas["JoinRoomResult"];

// Màn hình phòng, dùng chung cho host và viewer. Ba kết nối độc lập:
//   1. REST  POST /rooms/{id}/join → vai trò + token LiveKit (server quyết định ai được phát hình)
//   2. WebRTC tới LiveKit bằng token đó → video/audio
//   3. WebSocket tới backend → chat
// Một kết nối lỗi không kéo theo kết nối khác (VD chat rớt nhưng vẫn xem được video).
export function RoomScreen({ roomId }: { roomId: string }) {
  const token = useRequireAuth();
  const router = useRouter();
  const [joined, setJoined] = useState<Joined | null>(null);
  const [error, setError] = useState<unknown>(null);

  // (1) Vào phòng. Chạy lại khi quay lại route (Next.js ẩn route cũ thay vì gỡ bỏ, effect được tạo lại).
  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    api
      .POST("/rooms/{id}/join", { params: { path: { id: roomId } } })
      .then(unwrap)
      .then((res) => !cancelled && setJoined(res.data))
      .catch((err) => !cancelled && setError(err));
    return () => {
      cancelled = true;
    };
  }, [token, roomId]);

  if (!token) return null;
  if (error) {
    return (
      <div className="mt-10 text-center">
        <p className="mb-4 text-lg" role="alert">
          {errorMessage(error)}
        </p>
        <button onClick={() => router.push("/")} className="text-pink-400">
          Về trang chủ
        </button>
      </div>
    );
  }
  if (!joined) return <p className="text-neutral-500">Đang vào phòng…</p>;

  const isHost = joined.role === "HOST";

  async function endOrLeave() {
    try {
      if (isHost) unwrap(await api.POST("/rooms/{id}/end", { params: { path: { id: roomId } } }));
      else unwrap(await api.POST("/rooms/{id}/leave", { params: { path: { id: roomId } } }));
    } catch (err) {
      // Phòng đã kết thúc từ trước thì vẫn rời đi bình thường
      if (!(err instanceof ApiError && err.code === "ROOM_NOT_LIVE")) return setError(err);
    }
    router.push("/");
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[1fr_320px]">
      <section>
        <div className="mb-3 flex items-center justify-between gap-3">
          <div className="min-w-0">
            <h1 className="truncate text-xl font-bold" data-testid="room-title">
              {joined.room.title}
            </h1>
            <p className="text-sm text-neutral-400">{isHost ? "Bạn đang phát sóng" : "Đang xem"}</p>
          </div>
          <button
            onClick={endOrLeave}
            data-testid={isHost ? "end-live" : "leave-room"}
            className={`shrink-0 rounded-lg px-4 py-2 text-sm font-medium ${
              isHost ? "bg-red-600 text-white hover:bg-red-500" : "border border-white/10 hover:border-white/30"
            }`}
          >
            {isHost ? "Kết thúc live" : "Rời phòng"}
          </button>
        </div>

        {/* (2) Video. Host bật camera + mic ngay khi kết nối; viewer chỉ nhận.
            Kể cả viewer tự sửa code để bật camera, LiveKit vẫn từ chối vì token không có quyền publish. */}
        <LiveKitRoom
          serverUrl={joined.livekit_url}
          token={joined.livekit_token}
          connect
          video={isHost}
          audio={isHost}
          className="overflow-hidden rounded-xl bg-black"
        >
          <Stage hostId={joined.room.host_id} isHost={isHost} />
          <RoomAudioRenderer />
        </LiveKitRoom>
      </section>

      {/* (3) Chat qua WebSocket: chỉ mở sau khi đã vào phòng (server yêu cầu đang JOINED) */}
      <ChatPanel roomId={roomId} token={token} />
    </div>
  );
}
