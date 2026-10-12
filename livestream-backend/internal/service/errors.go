package service

import "fmt"

// Mã lỗi ổn định cho client (frontend xử lý theo mã, không theo câu chữ).
// Chỉ đặt mã riêng cho trường hợp client cần phản ứng khác nhau; còn lại client nhận mã chung
// theo loại lỗi (xem delivery/http). Đổi hoặc xóa một mã là thay đổi phá vỡ hợp đồng API.
const (
	CodeValidation            = "VALIDATION_ERROR"        // tô đỏ các ô trong Fields
	CodeRoomNotLive           = "ROOM_NOT_LIVE"           // "phòng đã kết thúc"
	CodeAlreadyLive           = "ALREADY_LIVE"            // host đang có phòng live khác
	CodeBannedFromRoom        = "BANNED_FROM_ROOM"        // đưa user ra khỏi phòng
	CodeIdempotencyKeyReused  = "IDEMPOTENCY_KEY_REUSED"  // lỗi lập trình phía app
	CodeUsernameTaken         = "USERNAME_TAKEN"          // ô username
	CodeEmailTaken            = "EMAIL_TAKEN"             // ô email
	CodeInvalidCredentials    = "INVALID_CREDENTIALS"     // sai email hoặc mật khẩu
	CodeAccountDisabled       = "ACCOUNT_DISABLED"        // tài khoản bị khóa/xóa
	CodeRewardBudgetExhausted = "REWARD_BUDGET_EXHAUSTED" // game provider: thử lại sau
)

// FieldError chỉ ra ô nhập nào sai và sai luật gì (VD "email" / "email", "password" / "min").
type FieldError struct {
	Field string `json:"field"`
	Rule  string `json:"rule"`
}

// Error là lỗi nghiệp vụ có mã. Kind là một trong các Err* (quyết định HTTP status và
// vẫn dùng được với errors.Is), Code là mã ổn định cho client.
type Error struct {
	Kind    error
	Code    string
	Message string
	Fields  []FieldError
}

func (e *Error) Error() string { return fmt.Sprintf("%v: %s", e.Kind, e.Message) }
func (e *Error) Unwrap() error { return e.Kind }

func newError(kind error, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// invalidField tạo lỗi validate cho một ô nhập.
func invalidField(field, rule, message string) *Error {
	return &Error{
		Kind:    ErrInvalidInput,
		Code:    CodeValidation,
		Message: message,
		Fields:  []FieldError{{Field: field, Rule: rule}},
	}
}

func errRoomNotLive() *Error {
	return newError(ErrConflict, CodeRoomNotLive, "room is not live")
}
