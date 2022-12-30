package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
	"github.com/zdgeier/jamsync/internal/server"
)

func main() {
	_, closer, err := server.New()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("The server is running...")

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done

	closer()
}
