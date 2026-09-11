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
	log.Fatal(http.ListenAndServe(":"+port, server.NewHandler(server.Config{ScenarioRoot: "/scenarios", DefaultsRoot: "/defaults"})))
}
