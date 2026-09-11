package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	ScenarioRoot string
	DefaultsRoot string
}

type fixture struct {
	mu       sync.Mutex
	root     string
	defaults map[string]map[string]string
	sessions map[string]*session
}
type session struct {
	scenario *scenario
	sequence map[int]int
	history  []historyEntry
}
type scenario struct {
	Name      string       `yaml:"name"`
	Responses []definition `yaml:"responses"`
}
type definition struct {
	Service   string         `yaml:"service"`
	Operation string         `yaml:"operation"`
	Match     map[string]any `yaml:"match"`
	Response  map[string]any `yaml:"response"`
	Error     *fixtureError  `yaml:"error"`
	Sequence  []result       `yaml:"sequence"`
}
type result struct {
	Response map[string]any `yaml:"response"`
	Error    *fixtureError  `yaml:"error"`
}
type fixtureError struct {
	Type    string `yaml:"type"`
	Message string `yaml:"message"`
	Status  int    `yaml:"status"`
}
type historyEntry struct {
	Service       string         `json:"service"`
	Operation     string         `json:"operation"`
	Parameters    map[string]any `json:"parameters"`
	Method        string         `json:"method"`
	Path          string         `json:"path"`
	Status        int            `json:"status"`
	Kind          string         `json:"kind"`
	ResponseIndex int            `json:"responseIndex"`
	ErrorType     string         `json:"errorType,omitempty"`
}

func NewHandler(c Config) http.Handler {
	root := c.ScenarioRoot
	if root == "" {
		root = "/scenarios"
	}
	defaultsRoot := c.DefaultsRoot
	if defaultsRoot == "" {
		defaultsRoot = "defaults"
	}
	return &fixture{root: root, defaults: loadDefaults(defaultsRoot), sessions: map[string]*session{}}
}

type defaultsDocument struct {
	Service string `yaml:"service"`
	Defaults struct {
		Response struct { Headers map[string]string `yaml:"headers"` } `yaml:"response"`
	} `yaml:"defaults"`
}

func loadDefaults(root string) map[string]map[string]string {
	result := map[string]map[string]string{}
	entries, err := os.ReadDir(root)
	if err != nil { return result }
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yml") && !strings.HasSuffix(entry.Name(), ".yaml")) { continue }
		data, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil { continue }
		var document defaultsDocument
		if yaml.Unmarshal(data, &document) == nil && document.Service != "" { result[document.Service] = document.Defaults.Response.Headers }
	}
	return result
}

func (f *fixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/__fixture/health" && r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.URL.Path == "/__fixture/sessions" && r.Method == http.MethodPost {
		f.create(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "__fixture" && parts[1] == "sessions" {
		id := parts[2]
		if len(parts) >= 4 && parts[3] == "aws" {
			f.aws(w, r, id, "/"+strings.Join(parts[4:], "/"))
			return
		}
		f.control(w, r, id, parts[3:])
		return
	}
	f.awsError(w, r, http.StatusBadRequest, "INVALID_FIXTURE_SESSION", "missing fixture session")
}

func (f *fixture) create(w http.ResponseWriter, r *http.Request) {
	id := uuid4()
	f.mu.Lock()
	f.sessions[id] = &session{sequence: map[int]int{}}
	f.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]string{"sessionId": id, "endpoint": "http://" + r.Host + "/__fixture/sessions/" + id + "/aws"})
}
func (f *fixture) control(w http.ResponseWriter, r *http.Request, id string, tail []string) {
	f.mu.Lock()
	s, ok := f.sessions[id]
	f.mu.Unlock()
	if !ok {
		controlError(w, http.StatusNotFound, "NOT_FOUND", "fixture session not found")
		return
	}
	if len(tail) == 0 && r.Method == http.MethodDelete {
		f.mu.Lock()
		delete(f.sessions, id)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(tail) != 1 {
		controlError(w, http.StatusNotFound, "NOT_FOUND", "control route not found")
		return
	}
	switch tail[0] {
	case "scenario":
		if r.Method != http.MethodPost {
			controlError(w, http.StatusNotFound, "NOT_FOUND", "control route not found")
			return
		}
		var input struct {
			Path string `json:"path"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			f.clear(s)
			controlError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON")
			return
		}
		sc, status, err := f.load(input.Path)
		if err != nil {
			f.clear(s)
			controlError(w, status, "INVALID_SCENARIO", err.Error())
			return
		}
		f.mu.Lock()
		s.scenario = sc
		s.sequence = map[int]int{}
		s.history = nil
		f.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"scenario": sc.Name, "loaded": true})
	case "reset":
		if r.Method != http.MethodPost {
			controlError(w, http.StatusNotFound, "NOT_FOUND", "control route not found")
			return
		}
		f.mu.Lock()
		s.sequence = map[int]int{}
		s.history = nil
		f.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{})
	case "requests":
		if r.Method != http.MethodGet {
			controlError(w, http.StatusNotFound, "NOT_FOUND", "control route not found")
			return
		}
		f.mu.Lock()
		h := append([]historyEntry(nil), s.history...)
		f.mu.Unlock()
		writeJSON(w, http.StatusOK, h)
	default:
		controlError(w, http.StatusNotFound, "NOT_FOUND", "control route not found")
	}
}
func (f *fixture) clear(s *session) {
	f.mu.Lock()
	s.scenario = nil
	s.sequence = map[int]int{}
	s.history = nil
	f.mu.Unlock()
}
func (f *fixture) load(p string) (*scenario, int, error) {
	if !strings.HasPrefix(p, "/scenarios/") || !strings.HasSuffix(p, ".yml") && !strings.HasSuffix(p, ".yaml") {
		return nil, http.StatusBadRequest, fmt.Errorf("scenario path must be under /scenarios")
	}
	name := filepath.Base(p)
	if name != strings.TrimPrefix(p, "/scenarios/") {
		return nil, http.StatusBadRequest, fmt.Errorf("scenario path escapes /scenarios")
	}
	b, err := os.ReadFile(filepath.Join(f.root, name))
	if os.IsNotExist(err) {
		return nil, http.StatusNotFound, fmt.Errorf("scenario not found")
	}
	if err != nil {
		return nil, http.StatusBadRequest, err
	}
	var sc scenario
	d := yaml.NewDecoder(strings.NewReader(string(b)))
	d.KnownFields(true)
	if err = d.Decode(&sc); err != nil {
		return nil, http.StatusBadRequest, err
	}
	if sc.Name == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("scenario name is required")
	}
	for _, def := range sc.Responses {
		if err := validate(def); err != nil {
			return nil, http.StatusBadRequest, err
		}
	}
	return &sc, http.StatusOK, nil
}
func validate(d definition) error {
	if d.Service == "" || d.Operation == "" {
		return fmt.Errorf("service and operation are required")
	}
	n := 0
	if d.Response != nil {
		n++
	}
	if d.Error != nil {
		n++
	}
	if d.Sequence != nil {
		n++
	}
	if n != 1 {
		return fmt.Errorf("exactly one response, error, or sequence is required")
	}
	if d.Sequence != nil && len(d.Sequence) == 0 {
		return fmt.Errorf("sequence must not be empty")
	}
	for _, x := range d.Sequence {
		if (x.Response == nil) == (x.Error == nil) {
			return fmt.Errorf("sequence item requires exactly one result")
		}
	}
	for _, m := range d.Match {
		if err := validateMatcher(m); err != nil {
			return err
		}
	}
	return nil
}
func validateMatcher(v any) error {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if len(m) != 1 {
		return fmt.Errorf("matcher must contain one key")
	}
	for k, x := range m {
		switch k {
		case "contains", "regex":
			if _, ok := x.(string); !ok {
				return fmt.Errorf("%s matcher requires string", k)
			}
			if k == "regex" {
				if _, e := regexp.Compile(x.(string)); e != nil {
					return e
				}
			}
		case "optional":
			return validateMatcher(x)
		default:
			return fmt.Errorf("unknown matcher %s", k)
		}
	}
	return nil
}

func (f *fixture) aws(w http.ResponseWriter, r *http.Request, id, stripped string) {
	f.mu.Lock()
	s, ok := f.sessions[id]
	f.mu.Unlock()
	if !ok {
		f.awsError(w, r, http.StatusBadRequest, "INVALID_FIXTURE_SESSION", "fixture session not found")
		return
	}
	service, op, params := normalize(r, stripped)
	f.mu.Lock()
	if s.scenario == nil {
		f.record(s, service, op, params, r, http.StatusInternalServerError, "unexpected", -1, "")
		f.mu.Unlock()
		f.awsError(w, r, http.StatusInternalServerError, "UNEXPECTED_AWS_REQUEST", "no matching fixture")
		return
	}
	for i, d := range s.scenario.Responses {
		if d.Service == service && d.Operation == op && matches(d.Match, params) {
			res, idx := pick(d, s.sequence, i)
			if res.Error != nil {
				st := res.Error.Status
				if st == 0 {
					st = http.StatusBadRequest
				}
				f.record(s, service, op, params, r, st, "error", idx, res.Error.Type)
				f.mu.Unlock()
				f.awsError(w, r, st, res.Error.Type, res.Error.Message)
				return
			}
			f.record(s, service, op, params, r, http.StatusOK, "response", idx, "")
			f.mu.Unlock()
			f.awsResponse(w, r, service, res.Response)
			return
		}
	}
	f.record(s, service, op, params, r, http.StatusInternalServerError, "unexpected", -1, "")
	f.mu.Unlock()
	f.awsError(w, r, http.StatusInternalServerError, "UNEXPECTED_AWS_REQUEST", "no matching fixture")
}
func pick(d definition, seq map[int]int, i int) (result, int) {
	if d.Sequence == nil {
		return result{Response: d.Response, Error: d.Error}, 0
	}
	n := seq[i]
	seq[i] = n + 1
	if n >= len(d.Sequence) {
		n = len(d.Sequence) - 1
	}
	return d.Sequence[n], n
}
func (f *fixture) record(s *session, svc, op string, p map[string]any, r *http.Request, status int, kind string, index int, etype string) {
	s.history = append(s.history, historyEntry{svc, op, p, r.Method, r.URL.Path, status, kind, index, etype})
}
func normalize(r *http.Request, path string) (string, string, map[string]any) {
	p := map[string]any{}
	body, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(body, &p)
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		a := strings.Split(target, ".")
		svc := strings.ToLower(a[0])
		if svc == "amazonsqs" {
			svc = "sqs"
		}
		return svc, a[len(a)-1], p
	}
	if strings.HasPrefix(path, "/model/") {
		x := strings.Split(strings.Trim(path, "/"), "/")
		if len(x) >= 3 {
			p["modelId"] = x[1]
			if x[2] == "invoke" {
				return "bedrock-runtime", "InvokeModel", p
			}
			if x[2] == "converse" {
				return "bedrock-runtime", "Converse", p
			}
		}
	}
	x := strings.Split(strings.Trim(path, "/"), "/")
	if len(x) >= 2 {
		p["Bucket"] = x[0]
		p["Key"] = strings.Join(x[1:], "/")
	}
	switch r.Method {
	case http.MethodGet:
		return "s3", "GetObject", p
	case http.MethodPut:
		return "s3", "PutObject", p
	case http.MethodHead:
		return "s3", "HeadObject", p
	}
	return "unknown", "unknown", p
}
func matches(ms map[string]any, p map[string]any) bool {
	for k, m := range ms {
		v, exists := p[k]
		if !match(m, v, exists) {
			return false
		}
	}
	return true
}
func match(m, v any, exists bool) bool {
	if x, ok := m.(map[string]any); ok {
		for k, inner := range x {
			switch k {
			case "optional":
				if !exists {
					return true
				}
				return match(inner, v, true)
			case "contains":
				return strings.Contains(fmt.Sprint(v), fmt.Sprint(inner))
			case "regex":
				return regexp.MustCompile(fmt.Sprint(inner)).MatchString(fmt.Sprint(v))
			}
		}
		return false
	}
	if !exists {
		return false
	}
	x := fmt.Sprint(m)
	if x == "*" {
		return true
	}
	return fmt.Sprint(v) == x
}
func (f *fixture) awsResponse(w http.ResponseWriter, r *http.Request, svc string, data map[string]any) {
	id := uuid4()
	for key, value := range f.defaults[svc] {
		w.Header().Set(key, strings.ReplaceAll(value, "${request_id}", id))
	}
	if svc == "s3" {
		for k, v := range data {
			if k == "body" || k == "headers" {
				continue
			}
			w.Header().Set(headerName(k), fmt.Sprint(v))
		}
		if hs, ok := data["headers"].(map[string]any); ok {
			for k, v := range hs {
				w.Header().Set(k, fmt.Sprint(v))
			}
		}
		w.Header().Set("x-amz-request-id", id)
		w.WriteHeader(http.StatusOK)
		if b, ok := data["body"]; ok {
			_, _ = io.WriteString(w, fmt.Sprint(b))
		}
		return
	}
	if svc == "secretsmanager" {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	} else if svc == "sqs" {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.Header().Set("x-amzn-requestid", id)
	if svc == "bedrock-runtime" {
		if body, ok := data["body"].(string); ok {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, body)
			return
		}
	}
	writeJSONBody(w, http.StatusOK, data)
}
func (f *fixture) awsError(w http.ResponseWriter, r *http.Request, status int, typ, msg string) {
	svc := ""
	if r.Header.Get("X-Amz-Target") != "" {
		svc, _, _ = normalize(r, "")
	} else if !strings.Contains(r.URL.Path, "/model/") {
		svc = "s3"
	}
	if svc == "s3" {
		w.Header().Set("Content-Type", "application/xml")
		w.Header().Set("x-amz-request-id", uuid4())
		w.WriteHeader(status)
		_, _ = io.WriteString(w, "<Error><Code>"+xmlEscape(typ)+"</Code><Message>"+xmlEscape(msg)+"</Message></Error>")
		return
	}
	if r.Header.Get("Content-Type") == "application/x-amz-json-1.0" {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	} else if r.Header.Get("Content-Type") == "application/x-amz-json-1.1" {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.Header().Set("x-amzn-requestid", uuid4())
	writeJSONBody(w, status, map[string]string{"__type": typ, "message": msg})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	writeJSONBody(w, status, v)
}
func writeJSONBody(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func controlError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}
func headerName(k string) string {
	if k == "ContentLength" {
		return "Content-Length"
	}
	return k
}
func xmlEscape(s string) string {
	b, _ := xml.Marshal(s)
	x := string(b)
	return strings.TrimSuffix(strings.TrimPrefix(x, "<string>"), "</string>")
}
func uuid4() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	x := hex.EncodeToString(b)
	return x[:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:]
}
