package main

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

func TestHealthcheckUsesConfiguredPortOrDefault(t *testing.T) {
	for _, tc := range []struct {
		name string
		port string
		env  string
	}{
		{name: "configured port", port: "", env: "configured"},
		{name: "default port", port: "4566"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:"+tc.port)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()

			request := make(chan string, 1)
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				request <- r.URL.Path
				w.WriteHeader(http.StatusOK)
			})}
			defer server.Close()
			go server.Serve(listener)

			command := exec.Command(os.Args[0], "-test.run=TestHealthcheckHelperProcess")
			command.Env = append(os.Environ(), "GO_WANT_HEALTHCHECK_HELPER=1")
			if tc.env == "configured" {
				command.Env = append(command.Env, "PORT="+strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
			} else {
				command.Env = append(command.Env, "PORT=")
			}
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("healthcheck failed: %v\n%s", err, output)
			}
			if got := <-request; got != "/__fixture/health" {
				t.Fatalf("healthcheck path = %q, want %q", got, "/__fixture/health")
			}
		})
	}
}

func TestHealthcheckHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HEALTHCHECK_HELPER") != "1" {
		return
	}
	main()
}
