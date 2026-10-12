package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"livestream-backend/internal/service"

	livekit "github.com/livekit/protocol/auth"
)

type roomPage struct {
	Data []struct {
		ID           string `json:"id"`
		HostUsername string `json:"host_username"`
	} `json:"data"`
	NextCursor *string `json:"next_cursor"`
}

func TestAPI_ListLiveRoomsPaginatesNewestFirst(t *testing.T) {
	a := newAPITest(t)
	var opened []string // theo thứ tự lên sóng
	var tokens []string
	for _, name := range []string{"host1", "host2", "host3"} {
		_, token := a.signUp(name)
		tokens = append(tokens, token)
		opened = append(opened, a.openRoom(token))
	}

	// Public: không cần token
	var p1 roomPage
	r := a.do("GET", "/api/v1/rooms?limit=2", "", nil, nil)
	if r.Status != http.StatusOK {
		t.Fatalf("page 1: %d %s", r.Status, r.Body)
	}
	r.decode(t, &p1)
	if len(p1.Data) != 2 || p1.NextCursor == nil {
		t.Fatalf("page 1 = %d rooms, next %v; want 2 rooms and a cursor", len(p1.Data), p1.NextCursor)
	}
	if p1.Data[0].ID != opened[2] || p1.Data[0].HostUsername != "host3" {
		t.Fatalf("newest room first: got %s (%s), want %s", p1.Data[0].ID, p1.Data[0].HostUsername, opened[2])
	}

	var p2 roomPage
	a.do("GET", "/api/v1/rooms?limit=2&cursor="+*p1.NextCursor, "", nil, nil).decode(t, &p2)
	if len(p2.Data) != 1 || p2.NextCursor != nil || p2.Data[0].ID != opened[0] {
		t.Fatalf("page 2 = %+v, want only the oldest room and no cursor", p2)
	}

	// Phòng đã kết thúc biến khỏi danh sách
	if r := a.do("POST", "/api/v1/rooms/"+opened[1]+"/end", tokens[1], nil, nil); r.Status != http.StatusOK {
		t.Fatalf("end room: %d %s", r.Status, r.Body)
	}
	var all roomPage
	a.do("GET", "/api/v1/rooms", "", nil, nil).decode(t, &all)
	for _, room := range all.Data {
		if room.ID == opened[1] {
			t.Fatal("ended room is still listed")
		}
	}
	if len(all.Data) != 2 {
		t.Fatalf("live rooms = %d, want 2", len(all.Data))
	}
}

func TestAPI_ListRoomsRejectsBadParams(t *testing.T) {
	a := newAPITest(t)
	for query, field := range map[string]string{
		"?cursor=not-a-cursor": "cursor",
		"?limit=1000":          "limit",
		"?status=ENDED":        "status",
	} {
		r := a.do("GET", "/api/v1/rooms"+query, "", nil, nil)
		wantStatusCode(t, r, http.StatusUnprocessableEntity, service.CodeValidation)
		if f := r.errorFields(); len(f) != 1 || f[0].Field != field {
			t.Errorf("%s: fields = %v, want %s", query, f, field)
		}
	}
}

// Quyền phát hình nằm TRONG token LiveKit do server ký. Giải mã token để chứng minh
// viewer không phát được hình/âm thanh/data, dù client có sửa code.
func TestAPI_JoinGivesLiveKitURLAndOnlyHostCanPublish(t *testing.T) {
	a := newAPITest(t)
	hostID, hostToken := a.signUp("host")
	viewerID, viewerToken := a.signUp("viewer")
	roomID := a.openRoom(hostToken)

	type joinResult struct {
		Data struct {
			Role         string `json:"role"`
			LiveKitURL   string `json:"livekit_url"`
			LiveKitToken string `json:"livekit_token"`
		} `json:"data"`
	}
	join := func(token string) joinResult {
		t.Helper()
		var out joinResult
		r := a.do("POST", "/api/v1/rooms/"+roomID+"/join", token, nil, nil)
		if r.Status != http.StatusOK {
			t.Fatalf("join: %d %s", r.Status, r.Body)
		}
		r.decode(t, &out)
		return out
	}
	grants := func(raw string) *livekit.ClaimGrants {
		t.Helper()
		v, err := livekit.ParseAPIToken(raw)
		if err != nil {
			t.Fatalf("parse livekit token: %v", err)
		}
		_, g, err := v.Verify(testLiveKitSecret)
		if err != nil {
			t.Fatalf("verify livekit token: %v", err)
		}
		return g
	}

	host := join(hostToken)
	viewer := join(viewerToken)

	if host.Data.LiveKitURL != testLiveKitURL || viewer.Data.LiveKitURL != testLiveKitURL {
		t.Fatalf("livekit_url = %q / %q, want %q", host.Data.LiveKitURL, viewer.Data.LiveKitURL, testLiveKitURL)
	}

	hg, vg := grants(host.Data.LiveKitToken), grants(viewer.Data.LiveKitToken)
	if host.Data.Role != "HOST" || !hg.Video.GetCanPublish() || hg.Identity != hostID.String() {
		t.Fatalf("host: role %s, canPublish %v, identity %s", host.Data.Role, hg.Video.GetCanPublish(), hg.Identity)
	}
	if viewer.Data.Role != "VIEWER" || vg.Video.GetCanPublish() || vg.Video.GetCanPublishData() || !vg.Video.GetCanSubscribe() {
		t.Fatalf("viewer: role %s, canPublish %v, canPublishData %v, canSubscribe %v",
			viewer.Data.Role, vg.Video.GetCanPublish(), vg.Video.GetCanPublishData(), vg.Video.GetCanSubscribe())
	}
	if vg.Identity != viewerID.String() || vg.Video.Room == "" {
		t.Fatalf("viewer token identity %q room %q", vg.Identity, vg.Video.Room)
	}
}

func TestCORS_OnlyAllowedOriginGetsHeaders(t *testing.T) {
	r := SetupRouter(Services{CORSAllowedOrigins: []string{testWebOrigin}})

	preflight := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/rooms", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "authorization, content-type, idempotency-key")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	ok := preflight(testWebOrigin)
	if ok.Code != http.StatusNoContent || ok.Header().Get("Access-Control-Allow-Origin") != testWebOrigin {
		t.Fatalf("allowed origin preflight: %d, Allow-Origin %q", ok.Code, ok.Header().Get("Access-Control-Allow-Origin"))
	}
	for _, h := range []string{"Authorization", "Content-Type", "Idempotency-Key"} {
		if !containsFold(ok.Header().Get("Access-Control-Allow-Headers"), h) {
			t.Errorf("Allow-Headers %q does not include %s", ok.Header().Get("Access-Control-Allow-Headers"), h)
		}
	}

	evil := preflight("https://evil.example")
	if got := evil.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unknown origin got Allow-Origin %q, want none (browser must block)", got)
	}
}

func TestHealthEndpoints(t *testing.T) {
	if got := serve(SetupRouter(Services{}), "/healthz"); got != http.StatusOK {
		t.Fatalf("/healthz = %d", got)
	}
	ready := SetupRouter(Services{Ready: func(context.Context) error { return nil }})
	if got := serve(ready, "/readyz"); got != http.StatusOK {
		t.Fatalf("/readyz healthy = %d, want 200", got)
	}
	down := SetupRouter(Services{Ready: func(context.Context) error { return errors.New("db down") }})
	if got := serve(down, "/readyz"); got != http.StatusServiceUnavailable {
		t.Fatalf("/readyz with DB down = %d, want 503", got)
	}
}

func containsFold(list, item string) bool {
	for _, part := range strings.Split(list, ",") {
		if strings.EqualFold(strings.TrimSpace(part), item) {
			return true
		}
	}
	return false
}
