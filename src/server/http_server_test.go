package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// These tests exercise the public HTTP boundary. Config makes the read-only
// /scenarios mount replaceable with a temporary directory in unit tests.
func TestHealthAndScenarioLoadServeSecretsManagerResponseAndHistory(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "secret.yml", `
name: secret
responses:
  - service: secretsmanager
    operation: GetSecretValue
    match:
      SecretId: test/db
    response:
      Name: test/db
      SecretString: '{"host":"db"}'
`)

	handler := NewHandler(Config{ScenarioRoot: scenarios})

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/__fixture/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", health.Code, http.StatusOK)
	}

	loaded := serveJSON(handler, http.MethodPost, "/__fixture/scenario", map[string]string{
		"path": "/scenarios/secret.yml",
	})
	if loaded.Code != http.StatusOK {
		t.Fatalf("load status = %d, body = %s", loaded.Code, loaded.Body.String())
	}

	aws := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"SecretId":"test/db"}`))
	aws.Header.Set("Content-Type", "application/x-amz-json-1.1")
	aws.Header.Set("X-Amz-Target", "secretsmanager.GetSecretValue")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, aws)
	if response.Code != http.StatusOK {
		t.Fatalf("GetSecretValue status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/x-amz-json-1.1" {
		t.Fatalf("Content-Type = %q, want aws-json-1.1 default", got)
	}
	if response.Header().Get("x-amzn-requestid") == "" {
		t.Fatal("GetSecretValue response did not include x-amzn-requestid")
	}
	var body map[string]any
	decodeJSON(t, response.Body, &body)
	if body["Name"] != "test/db" || body["SecretString"] != `{"host":"db"}` {
		t.Fatalf("GetSecretValue body = %#v", body)
	}

	history := httptest.NewRecorder()
	handler.ServeHTTP(history, httptest.NewRequest(http.MethodGet, "/__fixture/requests", nil))
	if history.Code != http.StatusOK {
		t.Fatalf("history status = %d, body = %s", history.Code, history.Body.String())
	}
	var entries []map[string]any
	decodeJSON(t, history.Body, &entries)
	if len(entries) != 1 {
		t.Fatalf("history length = %d, want 1: %#v", len(entries), entries)
	}
	entry := entries[0]
	if entry["service"] != "secretsmanager" || entry["operation"] != "GetSecretValue" || entry["kind"] != "response" || entry["status"] != float64(http.StatusOK) {
		t.Fatalf("history entry = %#v", entry)
	}
	parameters, ok := entry["parameters"].(map[string]any)
	if !ok || parameters["SecretId"] != "test/db" {
		t.Fatalf("history parameters = %#v", entry["parameters"])
	}
}

func TestSequenceResetRestartsSequenceWithoutDiscardingScenario(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "sequence.yml", `
name: sequence
responses:
  - service: sqs
    operation: SendMessage
    match:
      QueueUrl: "*"
    sequence:
      - error:
          type: ThrottlingException
          message: throttled
          status: 429
      - response:
          MessageId: message-1
`)
	handler := NewHandler(Config{ScenarioRoot: scenarios})
	if loaded := serveJSON(handler, http.MethodPost, "/__fixture/scenario", map[string]string{"path": "/scenarios/sequence.yml"}); loaded.Code != http.StatusOK {
		t.Fatalf("load status = %d, body = %s", loaded.Code, loaded.Body.String())
	}

	first := sendMessage(handler)
	if first.Code != http.StatusTooManyRequests {
		t.Fatalf("first SendMessage status = %d, want %d", first.Code, http.StatusTooManyRequests)
	}
	second := sendMessage(handler)
	if second.Code != http.StatusOK || !bytes.Contains(second.Body.Bytes(), []byte(`"MessageId":"message-1"`)) {
		t.Fatalf("second SendMessage = (%d, %s), want successful sequence response", second.Code, second.Body.String())
	}
	third := sendMessage(handler)
	if third.Code != http.StatusOK {
		t.Fatalf("sequence exhaustion status = %d, want last response to continue", third.Code)
	}

	reset := serveJSON(handler, http.MethodPost, "/__fixture/reset", nil)
	if reset.Code != http.StatusOK {
		t.Fatalf("reset status = %d, body = %s", reset.Code, reset.Body.String())
	}
	if afterReset := sendMessage(handler); afterReset.Code != http.StatusTooManyRequests {
		t.Fatalf("SendMessage after reset status = %d, want %d", afterReset.Code, http.StatusTooManyRequests)
	}

	history := httptest.NewRecorder()
	handler.ServeHTTP(history, httptest.NewRequest(http.MethodGet, "/__fixture/requests", nil))
	var entries []map[string]any
	decodeJSON(t, history.Body, &entries)
	if len(entries) != 1 || entries[0]["kind"] != "error" || entries[0]["errorType"] != "ThrottlingException" {
		t.Fatalf("history after reset = %#v", entries)
	}
}

func TestFailedScenarioLoadClearsScenarioAndUnexpectedRequestFailsFast(t *testing.T) {
	handler := NewHandler(Config{ScenarioRoot: t.TempDir()})
	failed := serveJSON(handler, http.MethodPost, "/__fixture/scenario", map[string]string{"path": "/outside.yml"})
	if failed.Code != http.StatusBadRequest {
		t.Fatalf("outside scenario status = %d, want %d", failed.Code, http.StatusBadRequest)
	}

	response := sendMessage(handler)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected request status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("UNEXPECTED_AWS_REQUEST")) {
		t.Fatalf("unexpected request body = %s", response.Body.String())
	}
}

func sendMessage(handler http.Handler) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"QueueUrl":"https://sqs.us-east-1.amazonaws.com/123456789012/test","MessageBody":"hello"}`))
	request.Header.Set("Content-Type", "application/x-amz-json-1.0")
	request.Header.Set("X-Amz-Target", "AmazonSQS.SendMessage")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func serveJSON(handler http.Handler, method, path string, payload any) *httptest.ResponseRecorder {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			panic(err)
		}
		body = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func writeScenario(t *testing.T, root, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func decodeJSON(t *testing.T, body io.Reader, target any) {
	t.Helper()
	if err := json.NewDecoder(body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
