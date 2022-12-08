package web

import (
	"encoding/gob"
	"flag"
	"log"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	api "github.com/zdgeier/jamsync/internal/web/api/projects"
	"github.com/zdgeier/jamsync/internal/web/authenticator"
	"github.com/zdgeier/jamsync/internal/web/callback"
	"github.com/zdgeier/jamsync/internal/web/login"
	"github.com/zdgeier/jamsync/internal/web/logout"
	"github.com/zdgeier/jamsync/internal/web/middleware"
	"github.com/zdgeier/jamsync/internal/web/user"
)

var serverAddr = flag.String("addr", "localhost:14357", "The server address in the format of host:port")

// New registers the routes and returns the router.
func New(auth *authenticator.Authenticator) *gin.Engine {
	router := gin.Default()

	// To store custom types in our cookies,
	// we must first register them using gob.Register
	gob.Register(map[string]interface{}{})

	store := cookie.NewStore([]byte("secret"))
	router.Use(sessions.Sessions("auth-session", store))

	router.Static("/public", "static")
	router.LoadHTMLGlob("template/*")

	router.GET("/", func(ctx *gin.Context) {
		ctx.HTML(http.StatusOK, "home.html", nil)
	})
	router.GET("/favicon.ico", func(ctx *gin.Context) {
		ctx.Header("Content-Type", "image/svg+xml")
		ctx.File("static/grapes_optimized_purple.svg")
	})
	router.GET("/robots.txt", func(ctx *gin.Context) {
		ctx.Header("Content-Type", "text/plain")
		ctx.File("static/robots.txt")
	})
	router.GET("/login", login.Handler(auth))
	router.GET("/callback", callback.Handler(auth))
	router.GET("/logout", logout.Handler)
	router.GET("/user", middleware.IsAuthenticated, user.Handler)

	conn, err := grpc.Dial(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Panicf("could not connect to jamsync server: %s", err)
	}

	client := jamsyncpb.NewJamsyncAPIClient(conn)

	router.GET("/api/projects", middleware.IsAuthenticated, api.ProjectsHandler(client))

	return router
}
