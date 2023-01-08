package main

import (
	"log"
	"net/http"

	"github.com/zdgeier/jamsync/internal/jamenv"
	"github.com/zdgeier/jamsync/internal/web"
	"github.com/zdgeier/jamsync/internal/web/authenticator"
)

func main() {
	err := jamenv.LoadFile()
	if err != nil {
		log.Panic(err)
	}

	auth, err := authenticator.New()
	if err != nil {
		log.Fatalf("Failed to initialize the authenticator: %v", err)
	}

	rtr := web.New(auth)

	log.Print("Server listening on http://localhost:8081/")
	if err := http.ListenAndServe("0.0.0.0:8081", rtr); err != nil {
		log.Fatalf("There was an error with the http server: %v", err)
	}
}
