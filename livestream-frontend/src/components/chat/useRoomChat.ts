"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import type { Schemas } from "@/lib/api/client";
import { WS_URL } from "@/lib/config";

export type ChatMessage = Schemas["ChatMessage"];
export type ChatStatus = "connecting" | "open" | "closed";

const MAX_MESSAGES = 200; // giữ tối đa N tin để bộ nhớ không phình khi phòng chat nhiều

// Kết nối WebSocket chat của một phòng.
// - Kết nối trong effect và đóng ở hàm dọn dẹp: Next.js ẩn route khi chuyển trang (không gỡ bỏ),
//   effect được dọn khi route bị ẩn, nên không để lại kết nối treo.
// - Tin của chính mình cũng đi vòng qua server rồi mới hiện, để chắc chắn đã được lưu.
// - Chưa tự kết nối lại khi rớt mạng và chưa lấy lại tin bị lỡ: làm ở bước 7.
export function useRoomChat(roomId: string, token: string) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [status, setStatus] = useState<ChatStatus>("connecting");
  const socketRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    // Trình duyệt không gửi được header Authorization khi mở WebSocket → token đi qua query
    // (bước 8 thay bằng vé dùng một lần, để token thật không nằm trên URL).
    const ws = new WebSocket(`${WS_URL}/api/v1/ws/chat/${roomId}?access_token=${encodeURIComponent(token)}`);
    socketRef.current = ws;

    ws.onopen = () => setStatus("open");
    ws.onclose = () => setStatus("closed");
    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data) as ChatMessage;
      setMessages((prev) => [...prev, msg].slice(-MAX_MESSAGES));
    };

    return () => {
      socketRef.current = null;
      ws.close();
    };
  }, [roomId, token]);

  const send = useCallback((content: string) => {
    const text = content.trim();
    const ws = socketRef.current;
    if (!text || !ws || ws.readyState !== WebSocket.OPEN) return false;
    ws.send(JSON.stringify({ content: text }));
    return true;
  }, []);

  return { messages, status, send };
}
