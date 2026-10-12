package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"livestream-backend/internal/auth"
	"livestream-backend/internal/service"
	"livestream-backend/internal/testutil"

	"github.com/gin-gonic/gin"
)

func serviceCodes() []string {
	return []string{
		service.CodeValidation, service.CodeRoomNotLive, service.CodeAlreadyLive, service.CodeBannedFromRoom,
		service.CodeIdempotencyKeyReused, service.CodeUsernameTaken, service.CodeEmailTaken,
		service.CodeInvalidCredentials, service.CodeAccountDisabled, service.CodeRewardBudgetExhausted,
	}
}

func serve(r *gin.Engine, path string) int {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Code
}

// ---------- Định dạng lỗi chung ----------

func TestAPI_MalformedJSONIs400(t *testing.T) {
	a := newAPITest(t)
	r := a.do("POST", "/api/v1/auth/register", "", `{"username": `, nil)
	wantStatusCode(t, r, http.StatusBadRequest, CodeMalformedRequest)
}

func TestAPI_ValidationErrorListsFieldsByJSONName(t *testing.T) {
	a := newAPITest(t)
	r := a.do("POST", "/api/v1/auth/register", "", map[string]any{"username": "alice", "email": "not-an-email"}, nil)
	wantStatusCode(t, r, http.StatusUnprocessableEntity, service.CodeValidation)

	got := map[string]string{}
	for _, f := range r.errorFields() {
		got[f.Field] = f.Rule
	}
	if got["email"] != "email" || got["password"] != "required" {
		t.Fatalf("fields = %v, want email:email and password:required", got)
	}
}

func TestAPI_ServiceValidationPointsToField(t *testing.T) {
	a := newAPITest(t)
	r := a.do("POST", "/api/v1/auth/register", "", map[string]any{
		"username": "alice", "email": "alice@test.local", "password": "short",
	}, nil)
	wantStatusCode(t, r, http.StatusUnprocessableEntity, service.CodeValidation)
	if f := r.errorFields(); len(f) != 1 || f[0].Field != "password" {
		t.Fatalf("fields = %v, want password", f)
	}
}

func TestAPI_UnknownRouteAndMethodUseErrorFormat(t *testing.T) {
	a := newAPITest(t)
	wantStatusCode(t, a.do("GET", "/api/v1/nope", "", nil, nil), http.StatusNotFound, CodeNotFound)
	wantStatusCode(t, a.do("DELETE", "/api/v1/auth/login", "", nil, nil), http.StatusMethodNotAllowed, CodeMethodNotAllowed)
}

func TestAPI_InvalidPathIDIs400(t *testing.T) {
	a := newAPITest(t)
	_, token := a.signUp("alice")
	wantStatusCode(t, a.do("GET", "/api/v1/rooms/not-a-uuid", token, nil, nil), http.StatusBadRequest, CodeMalformedRequest)
}

func TestAPI_TimestampsAreUTC(t *testing.T) {
	a := newAPITest(t)
	r := a.do("POST", "/api/v1/auth/register", "", map[string]any{
		"username": "alice", "email": "alice@test.local", "password": "password123",
	}, nil)
	var out struct {
		Data struct {
			CreatedAt string `json:"created_at"`
		} `json:"data"`
	}
	r.decode(t, &out)
	ts, err := time.Parse(time.RFC3339Nano, out.Data.CreatedAt)
	if err != nil {
		t.Fatalf("created_at %q is not RFC 3339: %v", out.Data.CreatedAt, err)
	}
	if _, offset := ts.Zone(); offset != 0 || out.Data.CreatedAt[len(out.Data.CreatedAt)-1] != 'Z' {
		t.Fatalf("created_at %q is not UTC", out.Data.CreatedAt)
	}
}

// ---------- Xác thực ----------

func TestAPI_AuthErrorsTellClientWhatToDo(t *testing.T) {
	a := newAPITest(t)
	userID, _ := a.signUp("alice")

	wantStatusCode(t, a.do("GET", "/api/v1/me", "", nil, nil), http.StatusUnauthorized, CodeAuthRequired)
	wantStatusCode(t, a.do("GET", "/api/v1/me", "garbage", nil, nil), http.StatusUnauthorized, CodeTokenInvalid)

	expired, _, err := auth.NewTokenIssuer(testJWTSecret, -time.Minute).Issue(userID)
	if err != nil {
		t.Fatalf("issue expired token: %v", err)
	}
	wantStatusCode(t, a.do("GET", "/api/v1/me", expired, nil, nil), http.StatusUnauthorized, CodeTokenExpired)

	forged, _, err := auth.NewTokenIssuer("another-secret-0123456789abcdef01234", time.Hour).Issue(userID)
	if err != nil {
		t.Fatalf("issue forged token: %v", err)
	}
	wantStatusCode(t, a.do("GET", "/api/v1/me", forged, nil, nil), http.StatusUnauthorized, CodeTokenInvalid)
}

func TestAPI_RegisterAndLoginErrors(t *testing.T) {
	a := newAPITest(t)
	a.signUp("alice")

	r := a.do("POST", "/api/v1/auth/register", "", map[string]any{
		"username": "ALICE", "email": "other@test.local", "password": "password123",
	}, nil)
	wantStatusCode(t, r, http.StatusConflict, service.CodeUsernameTaken)

	r = a.do("POST", "/api/v1/auth/register", "", map[string]any{
		"username": "bob", "email": "Alice@Test.Local", "password": "password123",
	}, nil)
	wantStatusCode(t, r, http.StatusConflict, service.CodeEmailTaken)

	r = a.do("POST", "/api/v1/auth/login", "", map[string]any{"email": "alice@test.local", "password": "wrong-password"}, nil)
	wantStatusCode(t, r, http.StatusUnauthorized, service.CodeInvalidCredentials)
}

// ---------- Phòng và quà ----------

func TestAPI_SecondLiveRoomIsAlreadyLive(t *testing.T) {
	a := newAPITest(t)
	_, token := a.signUp("host")
	a.openRoom(token)
	wantStatusCode(t, a.do("POST", "/api/v1/rooms", token, map[string]any{"title": "again"}, nil),
		http.StatusConflict, service.CodeAlreadyLive)
}

func TestAPI_SendGiftFlow(t *testing.T) {
	a := newAPITest(t)
	_, hostToken := a.signUp("host")
	viewerID, viewerToken := a.signUp("viewer")
	roomID := a.openRoom(hostToken)
	gift := testutil.CreateGift(t, a.pool, "Rose", 10)
	path := "/api/v1/rooms/" + roomID + "/gifts"
	body := map[string]any{"gift_id": gift, "quantity": 3}
	key := map[string]string{headerIdempotencyKey: "key-1"}

	// Thiếu Idempotency-Key → 422 chỉ ra header
	r := a.do("POST", path, viewerToken, body, nil)
	wantStatusCode(t, r, http.StatusUnprocessableEntity, service.CodeValidation)
	if f := r.errorFields(); len(f) != 1 || f[0].Field != headerIdempotencyKey {
		t.Fatalf("fields = %v, want %s", f, headerIdempotencyKey)
	}

	// Chưa có coin → 402 INSUFFICIENT_BALANCE
	wantStatusCode(t, a.do("POST", path, viewerToken, body, key), http.StatusPaymentRequired, CodeInsufficient)

	// Có coin → 200, gửi lại cùng key → replayed
	testutil.Topup(t, a.pool, viewerID, 1000)
	if r := a.do("POST", path, viewerToken, body, key); r.Status != http.StatusOK {
		t.Fatalf("send gift: %d %s", r.Status, r.Body)
	}
	var replay struct {
		Data struct {
			Replayed bool `json:"replayed"`
		} `json:"data"`
	}
	a.do("POST", path, viewerToken, body, key).decode(t, &replay)
	if !replay.Data.Replayed {
		t.Fatal("same Idempotency-Key was not replayed")
	}

	// Cùng key, khác nội dung → 409 IDEMPOTENCY_KEY_REUSED
	wantStatusCode(t, a.do("POST", path, viewerToken, map[string]any{"gift_id": gift, "quantity": 4}, key),
		http.StatusConflict, service.CodeIdempotencyKeyReused)

	// Phòng kết thúc → 409 ROOM_NOT_LIVE
	if r := a.do("POST", "/api/v1/rooms/"+roomID+"/end", hostToken, nil, nil); r.Status != http.StatusOK {
		t.Fatalf("end room: %d %s", r.Status, r.Body)
	}
	wantStatusCode(t, a.do("POST", path, viewerToken, body, map[string]string{headerIdempotencyKey: "key-2"}),
		http.StatusConflict, service.CodeRoomNotLive)
}
