package http

import (
	"errors"
	"log"
	"net/http"
	"reflect"
	"strings"

	"livestream-backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// Định dạng lỗi duy nhất của API (xem components.schemas.Error trong api/openapi.yaml):
//
//	{"error": {"code": "INSUFFICIENT_BALANCE", "message": "...", "fields": [{"field": "email", "rule": "email"}]}}
//
// Client xử lý theo `code` (ổn định), không theo `message` (có thể đổi câu chữ).

// Mã lỗi chung, dùng khi service không gắn mã riêng. Mã riêng nằm ở service/errors.go.
const (
	CodeMalformedRequest = "MALFORMED_REQUEST" // 400: JSON hỏng, sai kiểu, ID sai định dạng
	CodeInvalidRequest   = "INVALID_REQUEST"   // 422: đúng định dạng nhưng vi phạm luật nghiệp vụ
	CodeAuthRequired     = "AUTH_REQUIRED"     // 401: không gửi token
	CodeTokenExpired     = "TOKEN_EXPIRED"     // 401: token hết hạn → làm mới token
	CodeTokenInvalid     = "TOKEN_INVALID"     // 401: token sai → về màn đăng nhập
	CodeUnauthorized     = "UNAUTHORIZED"      // 401: xác thực thất bại (VD chữ ký game sai)
	CodeInsufficient     = "INSUFFICIENT_BALANCE"
	CodeForbidden        = "FORBIDDEN"
	CodeNotFound         = "NOT_FOUND"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	CodeConflict         = "CONFLICT"
	CodeInternal         = "INTERNAL"
)

type errorDetail struct {
	Code    string               `json:"code"`
	Message string               `json:"message"`
	Fields  []service.FieldError `json:"fields,omitempty"`
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

// kinds: loại lỗi của service → HTTP status và mã chung.
var kinds = []struct {
	err    error
	status int
	code   string
}{
	{service.ErrInvalidInput, http.StatusUnprocessableEntity, CodeInvalidRequest},
	{service.ErrUnauthorized, http.StatusUnauthorized, CodeUnauthorized},
	{service.ErrInsufficientBalance, http.StatusPaymentRequired, CodeInsufficient},
	{service.ErrForbidden, http.StatusForbidden, CodeForbidden},
	{service.ErrNotFound, http.StatusNotFound, CodeNotFound},
	{service.ErrConflict, http.StatusConflict, CodeConflict},
}

func abortWithError(c *gin.Context, status int, detail errorDetail) {
	c.AbortWithStatusJSON(status, errorBody{Error: detail})
}

// writeError chuyển lỗi từ service sang response. Lỗi không thuộc loại nào ở trên là lỗi
// hệ thống: chỉ ghi log, client nhận thông báo chung để không lộ chi tiết nội bộ (SQL, tên bảng...).
func writeError(c *gin.Context, err error) {
	for _, k := range kinds {
		if !errors.Is(err, k.err) {
			continue
		}
		detail := errorDetail{Code: k.code, Message: strings.TrimPrefix(err.Error(), k.err.Error()+": ")}
		var appErr *service.Error
		if errors.As(err, &appErr) {
			detail = errorDetail{Code: appErr.Code, Message: appErr.Message, Fields: appErr.Fields}
		}
		abortWithError(c, k.status, detail)
		return
	}

	log.Printf("internal error on %s %s: %v", c.Request.Method, c.FullPath(), err)
	abortWithError(c, http.StatusInternalServerError, errorDetail{Code: CodeInternal, Message: "internal server error"})
}

// writeBindError xử lý lỗi khi đọc request:
//   - vi phạm luật validate (thiếu trường, sai định dạng email...) → 422 VALIDATION_ERROR kèm từng ô
//   - JSON hỏng, sai kiểu dữ liệu → 400 MALFORMED_REQUEST
func writeBindError(c *gin.Context, err error) {
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		fields := make([]service.FieldError, 0, len(verrs))
		for _, fe := range verrs {
			fields = append(fields, service.FieldError{Field: fe.Field(), Rule: fe.Tag()})
		}
		abortWithError(c, http.StatusUnprocessableEntity, errorDetail{
			Code:    service.CodeValidation,
			Message: "request has invalid fields",
			Fields:  fields,
		})
		return
	}
	abortWithError(c, http.StatusBadRequest, errorDetail{Code: CodeMalformedRequest, Message: "malformed request body"})
}

// useJSONFieldNames: lỗi validate báo tên trường theo JSON ("email") thay vì tên struct Go ("Email"),
// để frontend map thẳng vào ô nhập.
func useJSONFieldNames() {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return
	}
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			return f.Name
		}
		return name
	})
}

func notFound(c *gin.Context) {
	abortWithError(c, http.StatusNotFound, errorDetail{Code: CodeNotFound, Message: "route not found"})
}

func methodNotAllowed(c *gin.Context) {
	abortWithError(c, http.StatusMethodNotAllowed, errorDetail{Code: CodeMethodNotAllowed, Message: "method not allowed"})
}

// recovery: panic trong handler vẫn trả đúng định dạng lỗi (mặc định Gin trả 500 body rỗng).
func recovery(c *gin.Context, recovered any) {
	log.Printf("panic on %s %s: %v", c.Request.Method, c.FullPath(), recovered)
	abortWithError(c, http.StatusInternalServerError, errorDetail{Code: CodeInternal, Message: "internal server error"})
}
