package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"livestream-backend/api"
	"livestream-backend/internal/auth"
	"livestream-backend/internal/service"
	"livestream-backend/internal/testutil"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	testJWTSecret     = "test-jwt-secret-0123456789abcdef0123"
	testLiveKitURL    = "ws://livekit.test"
	testLiveKitSecret = "test-livekit-secret-0123456789abcdef"
	testWebOrigin     = "http://localhost:3000"
)

func init() { gin.SetMode(gin.TestMode) }

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(api.OpenAPISpec)
	if err != nil {
		t.Fatalf("load openapi.yaml: %v", err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("openapi.yaml is not a valid OpenAPI document: %v", err)
	}
	return doc
}

// apiTest chạy router thật trên DB test. Mọi response đi qua do() đều được đối chiếu với
// api/openapi.yaml: sai schema hoặc trả mã HTTP chưa khai báo thì test đỏ (contract test).
type apiTest struct {
	t      *testing.T
	pool   *pgxpool.Pool
	router *gin.Engine
	spec   *openapi3.T
	routes routers.Router
}

func newAPITest(t *testing.T) *apiTest {
	t.Helper()
	pool := testutil.Pool(t)
	testutil.Reset(t, pool)
	t.Cleanup(func() { testutil.AssertLedgerInvariants(t, pool) })

	spec := loadSpec(t)
	routes, err := gorillamux.NewRouter(spec)
	if err != nil {
		t.Fatalf("build spec router: %v", err)
	}

	tokens := auth.NewTokenIssuer(testJWTSecret, time.Hour)
	return &apiTest{
		t:    t,
		pool: pool,
		spec: spec,
		router: SetupRouter(Services{
			Tokens:   tokens,
			User:     service.NewUserService(pool, tokens),
			Gift:     service.NewGiftService(pool, 50),
			Room:     service.NewRoomService(pool, testLiveKitURL, "devkey", testLiveKitSecret),
			MiniGame: service.NewMiniGameService(pool, "test-minigame-secret"),

			CORSAllowedOrigins: []string{testWebOrigin},
		}),
		routes: routes,
	}
}

type response struct {
	Status int
	Body   []byte
}

// errorCode trả về error.code của body lỗi.
func (r response) errorCode() string {
	var b struct {
		Error struct {
			Code   string               `json:"code"`
			Fields []service.FieldError `json:"fields"`
		} `json:"error"`
	}
	_ = json.Unmarshal(r.Body, &b)
	return b.Error.Code
}

func (r response) errorFields() []service.FieldError {
	var b struct {
		Error struct {
			Fields []service.FieldError `json:"fields"`
		} `json:"error"`
	}
	_ = json.Unmarshal(r.Body, &b)
	return b.Error.Fields
}

func (r response) decode(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("decode %s: %v", r.Body, err)
	}
}

// do gửi request qua router và kiểm tra response theo hợp đồng.
func (a *apiTest) do(method, path, token string, body any, headers map[string]string) response {
	a.t.Helper()
	var raw []byte
	switch b := body.(type) {
	case nil:
	case string:
		raw = []byte(b) // dùng để gửi JSON hỏng
	default:
		var err error
		if raw, err = json.Marshal(b); err != nil {
			a.t.Fatalf("marshal body: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	if raw != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)
	res := response{Status: rec.Code, Body: rec.Body.Bytes()}
	a.checkContract(req, rec)
	return res
}

func (a *apiTest) checkContract(req *http.Request, rec *httptest.ResponseRecorder) {
	a.t.Helper()
	route, pathParams, err := a.routes.FindRoute(req)
	if err != nil {
		// Route không có trong spec (VD đường dẫn sai): body vẫn phải đúng schema Error
		var v any
		if jsonErr := json.Unmarshal(rec.Body.Bytes(), &v); jsonErr != nil {
			a.t.Fatalf("contract: %s %s returned non-JSON body: %s", req.Method, req.URL.Path, rec.Body.Bytes())
		}
		if verr := a.spec.Components.Schemas["Error"].Value.VisitJSON(v); verr != nil {
			a.t.Fatalf("contract: %s %s body does not match Error schema: %v", req.Method, req.URL.Path, verr)
		}
		return
	}

	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      route,
		},
		Status:  rec.Code,
		Header:  rec.Header(),
		Body:    io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
		Options: &openapi3filter.Options{IncludeResponseStatus: true}, // mã HTTP chưa khai báo cũng là vi phạm
	}
	if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
		a.t.Fatalf("contract: %s %s → %d does not match api/openapi.yaml: %v\nbody: %s",
			req.Method, req.URL.Path, rec.Code, err, rec.Body.Bytes())
	}
}

// signUp đăng ký + đăng nhập, trả về user id và access token.
func (a *apiTest) signUp(name string) (uuid.UUID, string) {
	a.t.Helper()
	if r := a.do("POST", "/api/v1/auth/register", "", map[string]any{
		"username": name, "email": name + "@test.local", "password": "password123",
	}, nil); r.Status != http.StatusCreated {
		a.t.Fatalf("register %s: %d %s", name, r.Status, r.Body)
	}
	r := a.do("POST", "/api/v1/auth/login", "", map[string]any{"email": name + "@test.local", "password": "password123"}, nil)
	if r.Status != http.StatusOK {
		a.t.Fatalf("login %s: %d %s", name, r.Status, r.Body)
	}
	var out struct {
		Data struct {
			AccessToken string    `json:"access_token"`
			UserID      uuid.UUID `json:"user_id"`
		} `json:"data"`
	}
	r.decode(a.t, &out)
	return out.Data.UserID, out.Data.AccessToken
}

// openRoom: host mở phòng, trả về room id.
func (a *apiTest) openRoom(token string) string {
	a.t.Helper()
	r := a.do("POST", "/api/v1/rooms", token, map[string]any{"title": "live"}, nil)
	if r.Status != http.StatusCreated {
		a.t.Fatalf("open room: %d %s", r.Status, r.Body)
	}
	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	r.decode(a.t, &out)
	return out.Data.ID
}

func wantStatusCode(t *testing.T, r response, status int, code string) {
	t.Helper()
	if r.Status != status || r.errorCode() != code {
		t.Fatalf("got %d %q, want %d %q\nbody: %s", r.Status, r.errorCode(), status, code, r.Body)
	}
}
