"use client";

import { VideoTrack, isTrackReference, useConnectionState, useTracks } from "@livekit/components-react";
import { ConnectionState, Track } from "livekit-client";

// Khung video chính: hiển thị camera của HOST (identity trong token LiveKit = user id, do server đặt).
export function Stage({ hostId, isHost }: { hostId: string; isHost: boolean }) {
  const connection = useConnectionState();
  const cameras = useTracks([Track.Source.Camera], { onlySubscribed: false });
  const hostCamera = cameras.find((t) => isTrackReference(t) && t.participant.identity === hostId);

  return (
    <div className="relative aspect-video w-full bg-black">
      {hostCamera && isTrackReference(hostCamera) ? (
        <VideoTrack
          trackRef={hostCamera}
          data-testid="stage-video"
          className={`h-full w-full object-contain ${isHost ? "-scale-x-100" : ""}`}
        />
      ) : (
        <div className="flex h-full items-center justify-center text-neutral-500" data-testid="stage-waiting">
          {connection === ConnectionState.Connected
            ? isHost
              ? "Đang bật camera…"
              : "Đang chờ hình từ host…"
            : "Đang kết nối…"}
        </div>
      )}
      <span className="absolute left-3 top-3 rounded bg-red-600 px-2 py-0.5 text-xs font-bold text-white">LIVE</span>
      <span className="absolute right-3 top-3 rounded bg-black/60 px-2 py-0.5 text-xs text-neutral-300" data-testid="connection-state">
        {connection}
      </span>
    </div>
  );
}
