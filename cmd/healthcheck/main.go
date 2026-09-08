package main

import (
	"net/http"
	"os"
)

func main() {
	response, err := http.Get("http://127.0.0.1:4566/__fixture/health")
	if err != nil || response.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	_ = response.Body.Close()
}
