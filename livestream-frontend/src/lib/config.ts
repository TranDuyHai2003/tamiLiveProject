// Biến NEXT_PUBLIC_* được Next.js thay bằng giá trị thật lúc build. Phải viết đúng
// `process.env.NEXT_PUBLIC_API_URL` (không dùng process.env[tên]) thì mới được thay.
export const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

// WebSocket dùng cùng host với API: http → ws, https → wss
export const WS_URL = API_URL.replace(/^http/, "ws");
