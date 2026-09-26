package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireKey(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	cases := []struct {
		name, configured, header string
		want                     int
	}{
		{"disabled", "", "Bearer " + key, http.StatusNotFound},
		{"missing", key, "", http.StatusUnauthorized},
		{"wrong", key, "Bearer " + strings.Repeat("x", 32), http.StatusUnauthorized},
		{"not bearer", key, key, http.StatusUnauthorized},
		{"correct", key, "Bearer " + key, http.StatusOK},
	}
	gin.SetMode(gin.TestMode)
	for _, tc := range cases {
		r := gin.New()
		r.GET("/x", RequireKey(tc.configured), func(c *gin.Context) { c.Status(http.StatusOK) })
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if tc.header != "" {
			req.Header.Set("Authorization", tc.header)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, w.Code, tc.want)
		}
	}
}

func TestValidRef(t *testing.T) {
	good := []string{"claims:ticket:123:public", "a", "x_y-z:0", strings.Repeat("a", 128)}
	bad := []string{"", "Claims:ticket", "ticket 1", "ticket/1", "ตั๋ว", strings.Repeat("a", 129)}
	for _, r := range good {
		if err := ValidRef(r); err != nil {
			t.Errorf("ValidRef(%q) = %v", r, err)
		}
	}
	for _, r := range bad {
		if err := ValidRef(r); err == nil {
			t.Errorf("ValidRef(%q) accepted", r)
		}
	}
}
