package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// These tests specify the public HTTP boundary. They deliberately use the
// session endpoint shape that AWS SDK clients receive; a custom session header
// must not be accepted as a substitute.
func TestSessionLifecycleRoutesStateIndependentlyAndRejectsDestroyedSession(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "first.yml", `
name: first
responses:
  - service: secretsmanager
    operation: GetSecretValue
    match: { SecretId: test/db }
    response: { Name: test/db, SecretString: first }
`)
	writeScenario(t, scenarios, "second.yml", `
name: second
responses:
  - service: secretsmanager
    operation: GetSecretValue
    match: { SecretId: test/db }
    response: { Name: test/db, SecretString: second }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios})

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/__fixture/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", health.Code)
	}

	first := createSession(t, handler)
	second := createSession(t, handler)
	loadScenario(t, handler, first, "/scenarios/first.yml")
	loadScenario(t, handler, second, "/scenarios/second.yml")

	if got := secret(t, handler, first, "test/db"); !bytes.Contains(got.Body.Bytes(), []byte(`"SecretString":"first"`)) {
		t.Fatalf("first session response = %s", got.Body.String())
	}
	if got := secret(t, handler, second, "test/db"); !bytes.Contains(got.Body.Bytes(), []byte(`"SecretString":"second"`)) {
		t.Fatalf("second session response = %s", got.Body.String())
	}
	assertHistory(t, handler, first, 1, "response")
	assertHistory(t, handler, second, 1, "response")

	destroyed := serveJSON(handler, http.MethodDelete, sessionControlPath(first, ""), nil)
	if destroyed.Code != http.StatusNoContent {
		t.Fatalf("destroy status = %d, body = %s", destroyed.Code, destroyed.Body.String())
	}
	if got := serveJSON(handler, http.MethodGet, sessionControlPath(first, "requests"), nil); got.Code != http.StatusNotFound {
		t.Fatalf("destroyed session history status = %d, want 404", got.Code)
	}
	assertInvalidJSONSession(t, secret(t, handler, first, "test/db"))

	// A normal AWS-shaped request outside the session endpoint is not allowed.
	missing := jsonRequest(http.MethodPost, "/", `{"SecretId":"test/db"}`, "application/x-amz-json-1.1", "secretsmanager.GetSecretValue")
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, missing)
	assertInvalidJSONSession(t, missingResponse)
}

func TestSessionControlScenarioSequenceResetAndHistoryMetadata(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "sequence.yml", `
name: sequence
responses:
  - service: sqs
    operation: SendMessage
    match: { QueueUrl: "*" }
    sequence:
      - error: { type: ThrottlingException, message: throttled, status: 429 }
      - response: { MessageId: message-1 }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios})
	session := createSession(t, handler)
	loadScenario(t, handler, session, "/scenarios/sequence.yml")

	first := sendMessage(handler, session, "https://sqs.us-east-1.amazonaws.com/123/test", "hello")
	if first.Code != http.StatusTooManyRequests || !bytes.Contains(first.Body.Bytes(), []byte("ThrottlingException")) {
		t.Fatalf("first sequence result = (%d, %s)", first.Code, first.Body.String())
	}
	second := sendMessage(handler, session, "https://sqs.us-east-1.amazonaws.com/123/test", "hello")
	if second.Code != http.StatusOK || !bytes.Contains(second.Body.Bytes(), []byte(`"MessageId":"message-1"`)) {
		t.Fatalf("second sequence result = (%d, %s)", second.Code, second.Body.String())
	}
	third := sendMessage(handler, session, "https://sqs.us-east-1.amazonaws.com/123/test", "hello")
	if third.Code != http.StatusOK {
		t.Fatalf("sequence must repeat its final result, got (%d, %s)", third.Code, third.Body.String())
	}

	history := sessionHistory(t, handler, session)
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3: %#v", len(history), history)
	}
	firstEntry := history[0]
	if firstEntry["service"] != "sqs" || firstEntry["operation"] != "SendMessage" || firstEntry["method"] != http.MethodPost || firstEntry["path"] == "" || firstEntry["kind"] != "error" || firstEntry["status"] != float64(http.StatusTooManyRequests) || firstEntry["responseIndex"] != float64(0) || firstEntry["errorType"] != "ThrottlingException" {
		t.Fatalf("first history entry = %#v", firstEntry)
	}
	if _, exposed := firstEntry["body"]; exposed {
		t.Fatalf("history must not expose response body: %#v", firstEntry)
	}
	parameters, ok := firstEntry["parameters"].(map[string]any)
	if !ok || parameters["QueueUrl"] != "https://sqs.us-east-1.amazonaws.com/123/test" || parameters["MessageBody"] != "hello" {
		t.Fatalf("history parameters = %#v", firstEntry["parameters"])
	}

	reset := serveJSON(handler, http.MethodPost, sessionControlPath(session, "reset"), nil)
	if reset.Code != http.StatusOK {
		t.Fatalf("reset status = %d, body = %s", reset.Code, reset.Body.String())
	}
	assertHistory(t, handler, session, 0, "")
	if afterReset := sendMessage(handler, session, "https://sqs.us-east-1.amazonaws.com/123/test", "hello"); afterReset.Code != http.StatusTooManyRequests {
		t.Fatalf("reset must preserve scenario and restart sequence, got %d", afterReset.Code)
	}
}

func TestMatchersUseFirstMatchAndRejectInvalidScenarioThenClearState(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "match.yml", `
name: match
responses:
  - service: secretsmanager
    operation: GetSecretValue
    match:
      SecretId: test/db
      ClientRequestToken:
        optional:
          regex: '^[a-z0-9-]+$'
    response: { SecretString: first }
  - service: secretsmanager
    operation: GetSecretValue
    match: { SecretId: test/db }
    response: { SecretString: fallback }
  - service: sqs
    operation: SendMessage
    match:
      QueueUrl: { regex: '^https://.+/queue$' }
      MessageBody: { contains: patientId }
    response: { MessageId: matched }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios})
	session := createSession(t, handler)
	loadScenario(t, handler, session, "/scenarios/match.yml")

	// Omitted optional input matches; this also proves response order wins.
	if got := secret(t, handler, session, "test/db"); !bytes.Contains(got.Body.Bytes(), []byte(`"SecretString":"first"`)) {
		t.Fatalf("optional matcher / first response result = %s", got.Body.String())
	}
	// Present optional input must satisfy its inner regex, so this request falls
	// through to the second definition.
	request := jsonRequest(http.MethodPost, sessionAWSPath(session, "/"), `{"SecretId":"test/db","ClientRequestToken":"UPPER"}`, "application/x-amz-json-1.1", "secretsmanager.GetSecretValue")
	fallback := httptest.NewRecorder()
	handler.ServeHTTP(fallback, request)
	if fallback.Code != http.StatusOK || !bytes.Contains(fallback.Body.Bytes(), []byte(`"SecretString":"fallback"`)) {
		t.Fatalf("optional regex fallback = (%d, %s)", fallback.Code, fallback.Body.String())
	}
	if got := sendMessage(handler, session, "https://sqs.us-east-1.amazonaws.com/123/queue", "contains patientId here"); got.Code != http.StatusOK || !bytes.Contains(got.Body.Bytes(), []byte(`"MessageId":"matched"`)) {
		t.Fatalf("contains / regex matcher result = (%d, %s)", got.Code, got.Body.String())
	}

	for name, contents := range map[string]string{
		"unknown-field.yml": `name: invalid
unknown: true
responses: []
`,
		"unknown-matcher.yml": `name: invalid
responses:
  - service: sqs
    operation: SendMessage
    match: { QueueUrl: { glob: "*" } }
    response: { MessageId: x }
`,
		"multiple-result.yml": `name: invalid
responses:
  - service: sqs
    operation: SendMessage
    response: { MessageId: x }
    error: { type: Bad }
`,
		"empty-sequence.yml": `name: invalid
responses:
  - service: sqs
    operation: SendMessage
    sequence: []
`,
		"invalid-yaml.yml": `name: invalid
responses: [
`,
	} {
		writeScenario(t, scenarios, name, contents)
		loaded := serveJSON(handler, http.MethodPost, sessionControlPath(session, "scenario"), map[string]string{"path": "/scenarios/" + name})
		if loaded.Code != http.StatusBadRequest {
			t.Fatalf("%s load status = %d, body = %s", name, loaded.Code, loaded.Body.String())
		}
		assertControlError(t, loaded)
		fallback := sendMessage(handler, session, "https://sqs.us-east-1.amazonaws.com/123/queue", "anything")
		if fallback.Code != http.StatusOK || !bytes.Contains(fallback.Body.Bytes(), []byte("fixture-message")) {
			t.Fatalf("%s must clear scenario and restore defaults: (%d, %s)", name, fallback.Code, fallback.Body.String())
		}
	}

	if outside := serveJSON(handler, http.MethodPost, sessionControlPath(session, "scenario"), map[string]string{"path": "/outside.yml"}); outside.Code != http.StatusBadRequest {
		t.Fatalf("outside scenario path = %d, want 400", outside.Code)
	} else {
		assertControlError(t, outside)
	}
	if missing := serveJSON(handler, http.MethodPost, sessionControlPath(session, "scenario"), map[string]string{"path": "/scenarios/does-not-exist.yml"}); missing.Code != http.StatusNotFound {
		t.Fatalf("missing scenario path = %d, want 404", missing.Code)
	} else {
		assertControlError(t, missing)
	}

	invalidJSON := httptest.NewRequest(http.MethodPost, sessionControlPath(session, "scenario"), strings.NewReader(`{"path":`))
	invalidJSON.Header.Set("Content-Type", "application/json")
	invalidJSONResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidJSONResponse, invalidJSON)
	if invalidJSONResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid scenario JSON status = %d, body = %s", invalidJSONResponse.Code, invalidJSONResponse.Body.String())
	}
	assertControlError(t, invalidJSONResponse)
}

func TestAWSResponseWriteDoesNotBlockOtherSession(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "object.yml", `
name: object
responses:
  - service: s3
    operation: GetObject
    match: { Bucket: test-bucket, Key: object }
    response: { body: slow }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios})
	first := createSession(t, handler)
	second := createSession(t, handler)
	loadScenario(t, handler, first, "/scenarios/object.yml")
	loadScenario(t, handler, second, "/scenarios/object.yml")

	slow := newBlockingResponseWriter()
	firstDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(slow, httptest.NewRequest(http.MethodGet, sessionAWSPath(first, "/test-bucket/object"), nil))
		close(firstDone)
	}()
	<-slow.writeStarted

	secondDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		secondDone <- s3Request(handler, second, http.MethodGet, "object", nil)
	}()

	timer := time.NewTimer(200 * time.Millisecond)
	select {
	case response := <-secondDone:
		if response.Code != http.StatusOK {
			t.Errorf("second session status = %d, want 200", response.Code)
		}
	case <-timer.C:
		t.Errorf("second session AWS request was blocked by another session's response write")
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	close(slow.unblock)
	<-firstDone
	select {
	case <-secondDone:
	default:
	}
}

type blockingResponseWriter struct {
	header       http.Header
	writeStarted chan struct{}
	unblock      chan struct{}
}

func newBlockingResponseWriter() *blockingResponseWriter {
	return &blockingResponseWriter{header: make(http.Header), writeStarted: make(chan struct{}), unblock: make(chan struct{})}
}

func (w *blockingResponseWriter) Header() http.Header { return w.header }
func (w *blockingResponseWriter) WriteHeader(int)     {}
func (w *blockingResponseWriter) Write(p []byte) (int, error) {
	close(w.writeStarted)
	<-w.unblock
	return len(p), nil
}

func TestFixtureErrorDefaultsToBadRequest(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "error.yml", `
name: error
responses:
  - service: secretsmanager
    operation: GetSecretValue
    match: { SecretId: missing }
    error: { type: ResourceNotFoundException, message: not found }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios})
	session := createSession(t, handler)
	loadScenario(t, handler, session, "/scenarios/error.yml")

	response := secret(t, handler, session, "missing")
	if response.Code != http.StatusBadRequest || response.Header().Get("Content-Type") != "application/x-amz-json-1.1" {
		t.Fatalf("default fixture error = (%d, %#v, %s)", response.Code, response.Header(), response.Body.String())
	}
	var body map[string]any
	decodeJSON(t, response.Body, &body)
	if body["__type"] != "ResourceNotFoundException" || body["message"] != "not found" {
		t.Fatalf("fixture error body = %#v", body)
	}
	assertHistory(t, handler, session, 1, "error")
}

func TestAllInitialAWSOperationsEncodeProtocolCompatibleResponses(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "all.yml", `
name: all
responses:
  - service: secretsmanager
    operation: GetSecretValue
    match: { SecretId: test/db }
    response: { Name: test/db, SecretString: secret }
  - service: sqs
    operation: SendMessage
    match: { QueueUrl: "*", MessageBody: hello }
    response: { MessageId: message-1 }
  - service: s3
    operation: GetObject
    match: { Bucket: test-bucket, Key: hello.txt }
    response:
      body: hello world
      headers: { Content-Type: text/plain, ETag: '"abc123"' }
  - service: s3
    operation: PutObject
    match: { Bucket: test-bucket, Key: upload.txt }
    response: { ETag: '"upload123"' }
  - service: s3
    operation: HeadObject
    match: { Bucket: test-bucket, Key: hello.txt }
    response: { ContentLength: 11, ETag: '"abc123"' }
  - service: bedrock-runtime
    operation: InvokeModel
    match: { modelId: test-model }
    response: { body: '{"completion":"fixture response"}' }
  - service: bedrock-runtime
    operation: Converse
    match: { modelId: test-model }
    response:
      output:
        message:
          role: assistant
          content: [{ text: fixture response }]
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios})
	session := createSession(t, handler)
	loadScenario(t, handler, session, "/scenarios/all.yml")

	secretResponse := secret(t, handler, session, "test/db")
	if secretResponse.Code != http.StatusOK || secretResponse.Header().Get("Content-Type") != "application/x-amz-json-1.1" {
		t.Fatalf("Secrets Manager protocol defaults = (%d, %#v)", secretResponse.Code, secretResponse.Header())
	}
	assertUUIDv4(t, secretResponse.Header().Get("x-amzn-requestid"))
	if got := sendMessage(handler, session, "https://sqs.us-east-1.amazonaws.com/123/queue", "hello"); got.Code != http.StatusOK || got.Header().Get("Content-Type") != "application/x-amz-json-1.0" || !bytes.Contains(got.Body.Bytes(), []byte(`"MessageId":"message-1"`)) {
		t.Fatalf("SQS aws-json-1.0 result = (%d, %#v, %s)", got.Code, got.Header(), got.Body.String())
	}

	get := s3Request(handler, session, http.MethodGet, "hello.txt", nil)
	if get.Code != http.StatusOK || get.Body.String() != "hello world" || get.Header().Get("Content-Type") != "text/plain" || get.Header().Get("ETag") != `"abc123"` {
		t.Fatalf("S3 GetObject rest-xml result = (%d, %#v, %s)", get.Code, get.Header(), get.Body.String())
	}
	assertUUIDv4(t, get.Header().Get("x-amz-request-id"))
	put := s3Request(handler, session, http.MethodPut, "upload.txt", strings.NewReader("upload"))
	if put.Code != http.StatusOK || put.Header().Get("ETag") != `"upload123"` {
		t.Fatalf("S3 PutObject rest-xml result = (%d, %#v)", put.Code, put.Header())
	}
	assertUUIDv4(t, put.Header().Get("x-amz-request-id"))
	head := s3Request(handler, session, http.MethodHead, "hello.txt", nil)
	if head.Code != http.StatusOK || head.Header().Get("ETag") != `"abc123"` || head.Header().Get("Content-Length") != "11" {
		t.Fatalf("S3 HeadObject rest-xml result = (%d, %#v)", head.Code, head.Header())
	}

	invoke := jsonRequest(http.MethodPost, sessionAWSPath(session, "/model/test-model/invoke"), `{}`, "application/json", "")
	invokeResponse := httptest.NewRecorder()
	handler.ServeHTTP(invokeResponse, invoke)
	if invokeResponse.Code != http.StatusOK || !bytes.Contains(invokeResponse.Body.Bytes(), []byte(`"completion":"fixture response"`)) {
		t.Fatalf("Bedrock InvokeModel rest-json result = (%d, %#v, %s)", invokeResponse.Code, invokeResponse.Header(), invokeResponse.Body.String())
	}
	assertUUIDv4(t, invokeResponse.Header().Get("x-amzn-requestid"))
	converse := jsonRequest(http.MethodPost, sessionAWSPath(session, "/model/test-model/converse"), `{"messages":[]}`, "application/json", "")
	converseResponse := httptest.NewRecorder()
	handler.ServeHTTP(converseResponse, converse)
	if converseResponse.Code != http.StatusOK || !bytes.Contains(converseResponse.Body.Bytes(), []byte(`"fixture response"`)) || converseResponse.Header().Get("x-amzn-requestid") == "" {
		t.Fatalf("Bedrock Converse rest-json result = (%d, %#v, %s)", converseResponse.Code, converseResponse.Header(), converseResponse.Body.String())
	}

	// REST/XML invalid-session errors use <Code>, rather than the AWS JSON __type.
	invalidS3 := s3Request(handler, "missing", http.MethodGet, "hello.txt", nil)
	if invalidS3.Code != http.StatusBadRequest || !bytes.Contains(invalidS3.Body.Bytes(), []byte("<Code>INVALID_FIXTURE_SESSION</Code>")) {
		t.Fatalf("rest-xml invalid session = (%d, %s)", invalidS3.Code, invalidS3.Body.String())
	}

	unexpectedS3 := s3Request(handler, session, http.MethodGet, "missing.txt", nil)
	if unexpectedS3.Code != http.StatusInternalServerError || !bytes.Contains(unexpectedS3.Body.Bytes(), []byte("<Code>UNEXPECTED_AWS_REQUEST</Code>")) {
		t.Fatalf("rest-xml unexpected request = (%d, %s)", unexpectedS3.Code, unexpectedS3.Body.String())
	}
	unexpectedBedrock := jsonRequest(http.MethodPost, sessionAWSPath(session, "/model/unknown/invoke"), `{}`, "application/json", "")
	unexpectedBedrockResponse := httptest.NewRecorder()
	handler.ServeHTTP(unexpectedBedrockResponse, unexpectedBedrock)
	if unexpectedBedrockResponse.Code != http.StatusInternalServerError || !bytes.Contains(unexpectedBedrockResponse.Body.Bytes(), []byte("UNEXPECTED_AWS_REQUEST")) {
		t.Fatalf("rest-json unexpected request = (%d, %s)", unexpectedBedrockResponse.Code, unexpectedBedrockResponse.Body.String())
	}
}

func createSession(t *testing.T, handler http.Handler) string {
	t.Helper()
	created := serveJSON(handler, http.MethodPost, "/__fixture/sessions", nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create session status = %d, body = %s", created.Code, created.Body.String())
	}
	var payload struct {
		SessionID string `json:"sessionId"`
		Endpoint  string `json:"endpoint"`
	}
	decodeJSON(t, created.Body, &payload)
	if payload.SessionID == "" || !strings.HasSuffix(payload.Endpoint, "/__fixture/sessions/"+payload.SessionID+"/aws") {
		t.Fatalf("session payload = %#v", payload)
	}
	return payload.SessionID
}

func mustNewHandler(t *testing.T, config Config) http.Handler {
	t.Helper()
	handler, err := NewHandler(config)
	if err != nil {
		t.Fatalf("NewHandler(%+v): %v", config, err)
	}
	return handler
}

func loadScenario(t *testing.T, handler http.Handler, sessionID, path string) {
	t.Helper()
	loaded := serveJSON(handler, http.MethodPost, sessionControlPath(sessionID, "scenario"), map[string]string{"path": path})
	if loaded.Code != http.StatusOK {
		t.Fatalf("load %s status = %d, body = %s", path, loaded.Code, loaded.Body.String())
	}
	var payload map[string]any
	decodeJSON(t, loaded.Body, &payload)
	if payload["loaded"] != true {
		t.Fatalf("load response = %#v", payload)
	}
}

func secret(t *testing.T, handler http.Handler, sessionID, secretID string) *httptest.ResponseRecorder {
	t.Helper()
	request := jsonRequest(http.MethodPost, sessionAWSPath(sessionID, "/"), `{"SecretId":"`+secretID+`"}`, "application/x-amz-json-1.1", "secretsmanager.GetSecretValue")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func sendMessage(handler http.Handler, sessionID, queueURL, message string) *httptest.ResponseRecorder {
	request := jsonRequest(http.MethodPost, sessionAWSPath(sessionID, "/"), `{"QueueUrl":"`+queueURL+`","MessageBody":"`+message+`"}`, "application/x-amz-json-1.0", "AmazonSQS.SendMessage")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func s3Request(handler http.Handler, sessionID, method, key string, body io.Reader) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, sessionAWSPath(sessionID, "/test-bucket/"+key), body)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func sessionControlPath(sessionID, suffix string) string {
	path := "/__fixture/sessions/" + sessionID
	if suffix != "" {
		path += "/" + suffix
	}
	return path
}

func sessionAWSPath(sessionID, suffix string) string {
	return "/__fixture/sessions/" + sessionID + "/aws" + suffix
}

func jsonRequest(method, path, payload, contentType, target string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(payload))
	request.Header.Set("Content-Type", contentType)
	if target != "" {
		request.Header.Set("X-Amz-Target", target)
	}
	return request
}

func assertInvalidJSONSession(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte("INVALID_FIXTURE_SESSION")) {
		t.Fatalf("invalid aws-json session = (%d, %s)", response.Code, response.Body.String())
	}
	var body map[string]any
	decodeJSON(t, response.Body, &body)
	if body["__type"] != "INVALID_FIXTURE_SESSION" {
		t.Fatalf("invalid aws-json error = %#v", body)
	}
}

func assertControlError(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]any
	decodeJSON(t, response.Body, &body)
	if body["error"] == "" || body["message"] == "" {
		t.Fatalf("control API error body = %#v", body)
	}
}

func assertUUIDv4(t *testing.T, value string) {
	t.Helper()
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(value) {
		t.Fatalf("request ID = %q, want UUID v4", value)
	}
}

func assertHistory(t *testing.T, handler http.Handler, sessionID string, wantLength int, wantKind string) {
	t.Helper()
	history := sessionHistory(t, handler, sessionID)
	if len(history) != wantLength {
		t.Fatalf("history length = %d, want %d: %#v", len(history), wantLength, history)
	}
	if wantKind != "" && history[0]["kind"] != wantKind {
		t.Fatalf("history kind = %#v, want %q", history[0], wantKind)
	}
}

func sessionHistory(t *testing.T, handler http.Handler, sessionID string) []map[string]any {
	t.Helper()
	history := serveJSON(handler, http.MethodGet, sessionControlPath(sessionID, "requests"), nil)
	if history.Code != http.StatusOK {
		t.Fatalf("history status = %d, body = %s", history.Code, history.Body.String())
	}
	var entries []map[string]any
	decodeJSON(t, history.Body, &entries)
	return entries
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
