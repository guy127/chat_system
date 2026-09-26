package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

const allowedOrigin = "https://claims.test"

func corsRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS([]string{allowedOrigin}))
	ok := func(c *gin.Context) { c.String(http.StatusOK, "ok") }
	r.GET("/rooms/:id", ok)
	r.PUT("/service/v1/rooms/:ref", ok)
	return r
}

func corsDo(method, path, origin string, preflight bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if preflight {
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	}
	w := httptest.NewRecorder()
	corsRouter().ServeHTTP(w, req)
	return w
}

func TestCORSPreflightAllowed(t *testing.T) {
	w := corsDo(http.MethodOptions, "/rooms/x/messages", allowedOrigin, true)
	h := w.Header()
	if w.Code != http.StatusNoContent || h.Get("Access-Control-Allow-Origin") != allowedOrigin {
		t.Fatalf("status %d, allow-origin %q", w.Code, h.Get("Access-Control-Allow-Origin"))
	}
	if h.Get("Access-Control-Allow-Headers") != "Authorization, Content-Type" {
		t.Fatalf("allow-headers %q", h.Get("Access-Control-Allow-Headers"))
	}
	if h.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("credentials must never be allowed")
	}
}

func TestCORSPreflightRejected(t *testing.T) {
	w := corsDo(http.MethodOptions, "/rooms/x", "https://evil.test", true)
	if w.Code != http.StatusForbidden || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("status %d, allow-origin %q", w.Code, w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSSimpleRequests(t *testing.T) {
	cases := []struct {
		name, path, origin, want string
	}{
		{"allowed origin", "/rooms/x", allowedOrigin, allowedOrigin},
		{"other origin", "/rooms/x", "https://evil.test", ""},
		{"same origin (no header)", "/rooms/x", "", ""},
		{"service API is server-to-server", "/service/v1/rooms/r", allowedOrigin, ""},
	}
	for _, tc := range cases {
		method := http.MethodGet
		if tc.path == "/service/v1/rooms/r" {
			method = http.MethodPut
		}
		w := corsDo(method, tc.path, tc.origin, false)
		if w.Code != http.StatusOK || w.Header().Get("Access-Control-Allow-Origin") != tc.want {
			t.Errorf("%s: status %d, allow-origin %q, want %q", tc.name, w.Code, w.Header().Get("Access-Control-Allow-Origin"), tc.want)
		}
	}
}
