package main

import (
	"github.com/irukasano/aws-sdk-fixture-server/src/server"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "4566"
	}
	handler, err := server.NewHandler(server.Config{ScenarioRoot: "/scenarios", DefaultsRoot: "/defaults"})
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
