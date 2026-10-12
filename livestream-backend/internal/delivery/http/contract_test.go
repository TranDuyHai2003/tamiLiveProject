package http

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Mọi route của router phải có trong api/openapi.yaml và ngược lại.
// Thêm route mà quên cập nhật hợp đồng (hoặc xóa route mà quên xóa trong hợp đồng) thì CI đỏ.
func TestRoutesMatchOpenAPISpec(t *testing.T) {
	spec := loadSpec(t)
	router := SetupRouter(Services{})

	const prefix = "/api/v1"
	ginParam := regexp.MustCompile(`:([a-zA-Z_]+)`)

	inRouter := map[string]bool{}
	for _, r := range router.Routes() {
		if !strings.HasPrefix(r.Path, prefix) {
			continue // /docs, /openapi.yaml không thuộc hợp đồng
		}
		path := ginParam.ReplaceAllString(strings.TrimPrefix(r.Path, prefix), "{$1}")
		inRouter[r.Method+" "+path] = true
	}

	inSpec := map[string]bool{}
	for path, item := range spec.Paths.Map() {
		for method := range item.Operations() {
			inSpec[method+" "+path] = true
		}
	}

	var missing, stale []string
	for k := range inRouter {
		if !inSpec[k] {
			missing = append(missing, k)
		}
	}
	for k := range inSpec {
		if !inRouter[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("routes missing from api/openapi.yaml:\n  %s", strings.Join(missing, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("api/openapi.yaml documents routes that do not exist:\n  %s", strings.Join(stale, "\n  "))
	}
}

// Mọi mã lỗi mà code có thể trả phải nằm trong enum Error.code của hợp đồng.
func TestErrorCodesAreDocumented(t *testing.T) {
	spec := loadSpec(t)
	documented := map[string]bool{}
	for _, v := range spec.Components.Schemas["Error"].Value.Properties["error"].Value.Properties["code"].Value.Enum {
		documented[v.(string)] = true
	}

	codes := []string{
		CodeMalformedRequest, CodeInvalidRequest, CodeAuthRequired, CodeTokenExpired, CodeTokenInvalid,
		CodeUnauthorized, CodeInsufficient, CodeForbidden, CodeNotFound, CodeMethodNotAllowed, CodeConflict, CodeInternal, CodeNotReady,
	}
	codes = append(codes, serviceCodes()...)
	for _, c := range codes {
		if !documented[c] {
			t.Errorf("error code %q is returned by the API but not listed in api/openapi.yaml", c)
		}
	}
}

func TestDocsServedOnlyWhenEnabled(t *testing.T) {
	on := SetupRouter(Services{ServeAPIDocs: true})
	off := SetupRouter(Services{ServeAPIDocs: false})

	for _, path := range []string{"/docs", "/openapi.yaml"} {
		if got := serve(on, path); got != http.StatusOK {
			t.Errorf("docs on: GET %s = %d, want 200", path, got)
		}
		if got := serve(off, path); got != http.StatusNotFound {
			t.Errorf("docs off: GET %s = %d, want 404", path, got)
		}
	}
}
