package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSESV2SendEmailDefaultResponseAndRequestHistory(t *testing.T) {
	handler := mustNewHandler(t, Config{})
	session := createSession(t, handler)

	response := sesv2Request(handler, session, "/v2/email/outbound-emails", `{
  "FromEmailAddress":"sender@example.test",
  "Destination":{"ToAddresses":["recipient@example.test"]},
  "Content":{"Simple":{"Subject":{"Data":"Simple subject"},"Body":{"Text":{"Data":"Simple body"}}}}
}`)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" || !bytes.Contains(response.Body.Bytes(), []byte(`"MessageId":"fixture-message"`)) {
		t.Fatalf("SES v2 default SendEmail = (%d, %#v, %s)", response.Code, response.Header(), response.Body.String())
	}

	history := sessionHistory(t, handler, session)
	if len(history) != 1 {
		t.Fatalf("SES v2 history length = %d, want 1: %#v", len(history), history)
	}
	entry := history[0]
	if entry["service"] != "sesv2" || entry["operation"] != "SendEmail" || entry["method"] != http.MethodPost || entry["path"] != sessionAWSPath(session, "/v2/email/outbound-emails") {
		t.Fatalf("SES v2 history metadata = %#v", entry)
	}
	parameters, ok := entry["parameters"].(map[string]any)
	if !ok || parameters["FromEmailAddress"] != "sender@example.test" {
		t.Fatalf("SES v2 history parameters = %#v", entry["parameters"])
	}
	content, ok := parameters["Content"].(map[string]any)
	if !ok || content["Simple"] == nil {
		t.Fatalf("SES v2 Simple content missing from history = %#v", parameters)
	}
}

func TestSESV2SendEmailScenarioErrorAndAllContentForms(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "ses.yml", `
name: ses
responses:
  - service: sesv2
    operation: SendEmail
    match: { FromEmailAddress: rejected@example.test }
    error: { type: MessageRejected, message: rejected by fixture, status: 400 }
  - service: sesv2
    operation: SendEmail
    match: { FromEmailAddress: raw@example.test }
    response: { MessageId: raw-message }
  - service: sesv2
    operation: SendEmail
    match: { FromEmailAddress: template@example.test }
    response: { MessageId: template-message }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios})
	session := createSession(t, handler)
	loadScenario(t, handler, session, "/scenarios/ses.yml")

	rejected := sesv2Request(handler, session, "/v2/email/outbound-emails", `{"FromEmailAddress":"rejected@example.test","Content":{"Simple":{"Subject":{"Data":"subject"},"Body":{"Text":{"Data":"body"}}}}}`)
	if rejected.Code != http.StatusBadRequest || rejected.Header().Get("Content-Type") != "application/json" || !bytes.Contains(rejected.Body.Bytes(), []byte(`"__type":"MessageRejected"`)) {
		t.Fatalf("SES v2 scenario error = (%d, %#v, %s)", rejected.Code, rejected.Header(), rejected.Body.String())
	}

	raw := sesv2Request(handler, session, "/v2/email/outbound-emails", `{"FromEmailAddress":"raw@example.test","Content":{"Raw":{"Data":"UmF3IG1lc3NhZ2U="}}}`)
	if raw.Code != http.StatusOK || !bytes.Contains(raw.Body.Bytes(), []byte(`"MessageId":"raw-message"`)) {
		t.Fatalf("SES v2 Raw content result = (%d, %s)", raw.Code, raw.Body.String())
	}
	template := sesv2Request(handler, session, "/v2/email/outbound-emails", `{"FromEmailAddress":"template@example.test","Content":{"Template":{"TemplateName":"welcome","TemplateData":"{\"name\":\"Ada\"}"}}}`)
	if template.Code != http.StatusOK || !bytes.Contains(template.Body.Bytes(), []byte(`"MessageId":"template-message"`)) {
		t.Fatalf("SES v2 Template content result = (%d, %s)", template.Code, template.Body.String())
	}

	history := sessionHistory(t, handler, session)
	if len(history) != 3 {
		t.Fatalf("SES v2 scenario history length = %d, want 3: %#v", len(history), history)
	}
	for i, form := range []string{"Simple", "Raw", "Template"} {
		parameters, ok := history[i]["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("SES v2 %s history parameters = %#v", form, history[i]["parameters"])
		}
		content, ok := parameters["Content"].(map[string]any)
		if !ok || content[form] == nil {
			t.Fatalf("SES v2 %s content missing from history = %#v", form, parameters)
		}
	}
}

func TestSESV2RejectsUnknownOperationsAndUnmatchedRequests(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "ses.yml", `
name: ses
responses:
  - service: sesv2
    operation: SendEmail
    match: { FromEmailAddress: matched@example.test }
    response: { MessageId: matched-message }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios})
	session := createSession(t, handler)
	loadScenario(t, handler, session, "/scenarios/ses.yml")

	for _, requestPath := range []string{"/v2/email/outbound-emails/unknown", "/v2/email/outbound-emails"} {
		response := sesv2Request(handler, session, requestPath, `{"FromEmailAddress":"unmatched@example.test","Content":{"Raw":{"Data":"dGVzdA=="}}}`)
		if response.Code != http.StatusInternalServerError || response.Header().Get("Content-Type") != "application/json" || !bytes.Contains(response.Body.Bytes(), []byte(`"__type":"UNEXPECTED_AWS_REQUEST"`)) {
			t.Fatalf("SES v2 unexpected request %s = (%d, %#v, %s)", requestPath, response.Code, response.Header(), response.Body.String())
		}
	}
}

func sesv2Request(handler http.Handler, sessionID, path, payload string) *httptest.ResponseRecorder {
	request := jsonRequest(http.MethodPost, sessionAWSPath(sessionID, path), payload, "application/json", "")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
