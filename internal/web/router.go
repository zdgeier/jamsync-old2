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

	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/jamenv"
	"github.com/zdgeier/jamsync/internal/web/api"
	"github.com/zdgeier/jamsync/internal/web/authenticator"
	"github.com/zdgeier/jamsync/internal/web/callback"
	"github.com/zdgeier/jamsync/internal/web/file"
	"github.com/zdgeier/jamsync/internal/web/files"
	"github.com/zdgeier/jamsync/internal/web/login"
	"github.com/zdgeier/jamsync/internal/web/logout"
	"github.com/zdgeier/jamsync/internal/web/middleware"
	"github.com/zdgeier/jamsync/internal/web/userprojects"
)

var useEnv = flag.Bool("useenv", false, "The server address in the format of host:port")

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
		session := sessions.Default(ctx)
		profile := session.Get("profile")
		ctx.HTML(http.StatusOK, "home.html", profile)
	})
	router.GET("/about", func(ctx *gin.Context) {
		session := sessions.Default(ctx)
		profile := session.Get("profile")
		ctx.HTML(http.StatusOK, "about.html", profile)
	})
	router.GET("/browse", func(ctx *gin.Context) {
		session := sessions.Default(ctx)
		profile := session.Get("profile")
		ctx.HTML(http.StatusOK, "browse.html", profile)
	})
	router.GET("/download", func(ctx *gin.Context) {
		session := sessions.Default(ctx)
		profile := session.Get("profile")
		ctx.HTML(http.StatusOK, "download.html", profile)
	})
	router.GET("/favicon.ico", func(ctx *gin.Context) {
		ctx.Header("Content-Type", "image/svg+xml")
		ctx.File("static/favicon.svg")
	})
	router.GET("/favicon.svg", func(ctx *gin.Context) {
		ctx.Header("Content-Type", "image/svg+xml")
		ctx.File("static/favicon.svg")
	})
	router.GET("/robots.txt", func(ctx *gin.Context) {
		ctx.Header("Content-Type", "text/plain")
		ctx.File("static/robots.txt")
	})

	flag.Parse()
	var serverAddr string
	if *useEnv {
		serverAddr = jamenv.PublicAPIAddress()
	} else {
		serverAddr = jamenv.LocalAPIAddress
	}

	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Panicf("could not connect to jamsync server: %s", err)
	}
	client := pb.NewJamsyncAPIClient(conn)

	router.GET("/login", login.Handler(auth))
	router.GET("/callback", callback.Handler(auth, client))
	router.GET("/logout", logout.Handler)

	router.GET("/api/projects", middleware.IsAuthenticated, api.UserProjectsHandler(client))
	router.GET("/api/projects/:projectName", middleware.IsAuthenticated, api.ProjectBrowseHandler(client))
	router.GET("/api/projects/:projectName/files/*path", middleware.IsAuthenticated, api.ProjectBrowseHandler(client))
	router.GET("/api/projects/:projectName/file/*path", middleware.IsAuthenticated, api.GetFileHandler(client))

	router.GET("/:username/projects", middleware.IsAuthenticated, userprojects.Handler)
	router.GET("/:username/:project/file/*path", middleware.IsAuthenticated, file.Handler)
	router.GET("/:username/:project/files/*path", middleware.IsAuthenticated, files.Handler)
	router.GET("/:username/:project", middleware.IsAuthenticated, files.Handler)
	return router
}
