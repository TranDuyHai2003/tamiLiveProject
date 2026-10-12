import { ApiError, type ErrorCode } from "./client";

// Thông báo cho người dùng, tra theo MÃ lỗi (ổn định) chứ không theo message của server.
const MESSAGES: Partial<Record<ErrorCode, string>> = {
  INVALID_CREDENTIALS: "Email hoặc mật khẩu không đúng.",
  ACCOUNT_DISABLED: "Tài khoản đã bị khóa.",
  USERNAME_TAKEN: "Tên đăng nhập đã có người dùng.",
  EMAIL_TAKEN: "Email đã được đăng ký.",
  ALREADY_LIVE: "Bạn đang có một phòng live khác.",
  ROOM_NOT_LIVE: "Phòng đã kết thúc.",
  BANNED_FROM_ROOM: "Bạn đã bị mời ra khỏi phòng này.",
  INSUFFICIENT_BALANCE: "Không đủ xu.",
  AUTH_REQUIRED: "Vui lòng đăng nhập.",
  TOKEN_EXPIRED: "Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại.",
  TOKEN_INVALID: "Phiên đăng nhập không hợp lệ, vui lòng đăng nhập lại.",
  NOT_FOUND: "Không tìm thấy.",
  FORBIDDEN: "Bạn không có quyền làm việc này.",
};

// Lỗi theo từng ô nhập (field + rule từ server)
const FIELD_RULES: Record<string, string> = {
  required: "Bắt buộc nhập.",
  email: "Email không hợp lệ.",
  format: "Định dạng không hợp lệ.",
  length: "Độ dài không hợp lệ.",
  unique: "Đã có người dùng.",
};

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return MESSAGES[err.code] ?? "Có lỗi xảy ra, vui lòng thử lại.";
  if (err instanceof TypeError) return "Không kết nối được máy chủ."; // fetch lỗi mạng / CORS
  return "Có lỗi xảy ra, vui lòng thử lại.";
}

export function fieldMessage(err: unknown, field: string): string | undefined {
  if (!(err instanceof ApiError)) return undefined;
  const rule = err.fieldRule(field);
  return rule ? (FIELD_RULES[rule] ?? "Không hợp lệ.") : undefined;
}
