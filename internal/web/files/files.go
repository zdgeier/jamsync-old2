package files

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
	ctx.HTML(http.StatusOK, "files.html", templateParams{
		Email: session.Get("email"),
	})
}
