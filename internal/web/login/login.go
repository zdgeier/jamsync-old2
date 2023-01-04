// web/app/login/login.go

package login

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/zdgeier/jamsync/internal/web/authenticator"
	"golang.org/x/oauth2"
)

// Handler for our login.
func Handler(auth *authenticator.Authenticator) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		state, err := generateRandomState()
		if err != nil {
			ctx.String(http.StatusInternalServerError, err.Error())
			return
		}

		// authorizationURL := fmt.Sprintf(
		// 	"https://%s/authorize?audience=api.jamsync.dev"+
		// 		"&scope=write:projects"+
		// 		"&response_type=code&client_id=%s"+
		// 		"&code_challenge=%s"+
		// 		"&code_challenge_method=S256&redirect_uri=%s&state=%s",
		// 	jamenv.Auth0Domain(), jamenv.Auth0ClientID(), auth.CodeVerifier.CodeChallengeS256(), jamenv.Auth0RedirectUrl(), state)

		// Save the state inside the session.
		session := sessions.Default(ctx)
		session.Set("state", state)
		if err := session.Save(); err != nil {
			ctx.String(http.StatusInternalServerError, err.Error())
			return
		}

		ctx.Redirect(http.StatusTemporaryRedirect, auth.AuthCodeURL(state, oauth2.SetAuthURLParam("audience", "api.jamsync.dev")))
		// ctx.Redirect(http.StatusTemporaryRedirect, authorizationURL)
	}
}

func generateRandomState() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	state := base64.StdEncoding.EncodeToString(b)

	return state, nil
}
