package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	jwtutil "dispatch/backend/pkg/jwtutil"
)

func Auth(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		const pfx = "Bearer "
		if !strings.HasPrefix(h, pfx) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer"})
			return
		}
		tok := strings.TrimSpace(strings.TrimPrefix(h, pfx))
		cl, err := jwtutil.Parse(secret, tok)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set("userID", cl.UserID)
		c.Next()
	}
}
