package httpx

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS lets pages from the listed origins call the REST API. JWTs travel in
// the Authorization header, so credentials are never allowed. /service/v1 is
// server-to-server and never answers cross-origin requests.
func CORS(origins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[o] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" || strings.HasPrefix(c.Request.URL.Path, "/service/") {
			c.Next()
			return
		}
		preflight := c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != ""
		h := c.Writer.Header()
		h.Add("Vary", "Origin")
		if !allowed[origin] {
			if preflight {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}
		h.Set("Access-Control-Allow-Origin", origin)
		if preflight {
			h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
