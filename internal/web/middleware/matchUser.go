// platform/middleware/isAuthenticated.go

package middleware

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// IsAuthenticated is a middleware that checks if
// the user has already been authenticated previously.
func MatchUserQuery(ctx *gin.Context) {
	profile := sessions.Default(ctx).Get("profile").(map[string]string)
	if profile != nil && profile["email"] == ctx.Query("email") {
		ctx.Next()
	} else {
		ctx.String(http.StatusUnauthorized, "Unauthorized")
	}
}
