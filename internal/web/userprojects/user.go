// web/app/user/user.go

package userprojects

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// Handler for our logged-in user page.
func Handler(ctx *gin.Context) {
	session := sessions.Default(ctx)

	type templateParams struct {
		Email interface{}
	}
	ctx.HTML(http.StatusOK, "user.html", templateParams{
		Email: session.Get("email"),
	})
}
