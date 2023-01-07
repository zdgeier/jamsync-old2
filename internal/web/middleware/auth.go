// platform/middleware/isAuthenticated.go

package middleware

import (
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func IsAuthenticated(ctx *gin.Context) {
	if sessions.Default(ctx).Get("email") == nil {
		ctx.Redirect(http.StatusSeeOther, "/")
	} else {
		ctx.Next()
	}
}

func Reauthenticate(ctx *gin.Context) {
	exp := sessions.Default(ctx).Get("exp")
	if exp != nil && exp.(time.Time).Before(time.Now()) {
		ctx.Redirect(http.StatusTemporaryRedirect, "/login")
	}
	ctx.Next()
}
