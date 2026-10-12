"use client";

import createClient, { type Middleware } from "openapi-fetch";

import { useAuth } from "@/lib/auth/store";
import { API_URL } from "@/lib/config";

import type { components, paths } from "./schema";

// Kiểu sinh tự động từ livestream-backend/api/openapi.yaml (`npm run gen:api`).
// Backend đổi hợp đồng mà frontend chưa cập nhật → lỗi biên dịch, không phải lỗi lúc chạy.
export type Schemas = components["schemas"];
export type ErrorCode = Schemas["Error"]["error"]["code"];
export type FieldError = NonNullable<Schemas["Error"]["error"]["fields"]>[number];

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: ErrorCode,
    message: string,
    readonly fields: FieldError[] = [],
  ) {
    super(message);
  }

  // Lỗi của một ô nhập cụ thể (VD "email"), để hiển thị dưới ô đó
  fieldRule(field: string): string | undefined {
    return this.fields.find((f) => f.field === field)?.rule;
  }
}

export const api = createClient<paths>({ baseUrl: `${API_URL}/api/v1` });

const authMiddleware: Middleware = {
  onRequest({ request }) {
    const token = useAuth.getState().token;
    if (token) request.headers.set("Authorization", `Bearer ${token}`);
    return request;
  },
  async onResponse({ response }) {
    // Token hết hạn hoặc sai → đăng xuất. (Bước 8: TOKEN_EXPIRED sẽ được làm mới thay vì đăng xuất.)
    if (response.status === 401) {
      const body = await response.clone().json().catch(() => null);
      const code = body?.error?.code as ErrorCode | undefined;
      if (code === "TOKEN_EXPIRED" || code === "TOKEN_INVALID") useAuth.getState().signOut();
    }
    return response;
  },
};
api.use(authMiddleware);

type Result<T> = {
  data?: T;
  error?: Schemas["Error"];
  response: Response;
};

// unwrap: trả về data nếu thành công, ném ApiError (có code, fields) nếu thất bại.
export function unwrap<T>({ data, error, response }: Result<T>): T {
  if (error) {
    throw new ApiError(response.status, error.error.code, error.error.message, error.error.fields ?? []);
  }
  if (data === undefined && response.status !== 204) {
    throw new ApiError(response.status, "INTERNAL", "empty response");
  }
  return data as T;
}
