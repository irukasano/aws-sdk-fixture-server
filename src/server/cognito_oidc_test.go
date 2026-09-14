package server

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const cognitoTestPrivateKey = `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDcL7BfrcuUMZl4
nnImRb24SCU+jfMHsuWP2o/gqWFLim7H+hRs8PBeeHx9RbvHf7pPekwiJzwClW0h
z+N4AQMLhynrNE4W/XmM3JcSz+jSCWz+beDi1c8kcCvKg36Ecys6NTGzhccFJPVI
RJPzjPB+fpxx0y3n59JEOgdWYOtjWc1TS6dtMheqmrtXQ2NqAqf2XnUWxpOVk6Ou
hcqr/NHkLLmmxtQnOYFnFICzHIN63B4JblIH0UeBu6JT3Kot84vwB2bonCCXLZVS
9VJrFZIrZeIwpY9PlVT0OaKV3ZougMRMHCD2dA0NPlfNjepWfOwJpbJIUzlW2ls5
qJCOPHOVAgMBAAECggEAAemQHv43B6FFDSnv7kRdmRCKi96KJWwr8XHoHTI1AoXA
Evb2Racm3Bf9ZmeJCy4hNut/zPqJdORJv49uRFT0UWqDgxcMzpPpAfL9UbaCKSc+
NExNnB3xrJ++YZ9+vwIl3MP1giYMf0KKg6D4DMqrYNqsqF7TLHGnZXXmwzZ+rnnN
eA7NQMbILkb0OwFXhmf0RDkEc9ueJdl51OLMZQ6voQbe0q5tb2Sd5NqqJvsLvAkS
/l8WvpjFCbWda/2FmmnIdWFGjrQBOXKgIncvbEHXcFY5bbYJvJE+9OTqL3TqnZEx
hZe7riv7JkpZdtsoHJYapTTlLqOQzuyISWYSmHDrgQKBgQDwpDFRqqgToqoSkrx2
C7JeIApSlEomHJ7vjgFnKVXy04lQa6BG6pdKXprxJZhwdO4to0fS9uu89qqPnVNs
epCCSMPs62apqFtzNNzvEPC9KpBD/plVaDUwKEhQCZdkBRwyAYQe+45mwGZQqqwp
rAmmq929ok7k276NPG3o1AIUhQKBgQDqPUiFFXVBnugny+parpqlJc60Mjfh7bOK
8lvf4yH5o8DNdTxygyG54S9ZM77wtyDtlwvooTdhfXXZUnI3bvDElnjveXKp6U+2
Z80POqHSC2uU3qInGgaWQGWX0qjx8P1bm4ZYMhloHKz2ORcbobfMmkE24cSInK/b
32I8dHbX0QKBgQDJ+SWj49aaVGbmm94uPfcBZWcElI3/mvoTGl83FMycuMuBgjPO
EcvVkb3+NI3TpXDbQTZhbPnDak0RqPyCCgUMNMKtMY7DSxkmgvIEfXVxcC4Rw7ky
o/owZz76XnKAcoGNvxQDZSPKPiYiAn3ppAmdqJa+OWJ6V62BoXymdHsieQKBgCbp
bU2mZeczxa8uwcy0qr36jN8EZKIkgan4XujPa4pz+IhpDFSdkhG15c60uOh2E4NE
j1biyTdpxxUGDDFHPbn6oTZu/2xYdMSBc+AuxkkFWqbNYTSLr0Jwii/xb91ZQk1g
ha3LDMtt0BHLJBqT/9t+WI0MbkCffttQvZn7Yu6xAoGBAKXYHwRKjqfNrWrI5aun
sNCZUr1rPNtqc0d/2hIXczrQn9f2eKkxL6Q0phTBOEMUDF6vHd4Xhpld8+3HTnwz
kzuUD8uCPLS4OTy/5Ai0VNdfaeXaJ5fEslYtIkFkT30Fp7o+aSrUO5rI9V6nkHSb
Ky2z/9f9qYSUbQbo+5guzqsn
-----END PRIVATE KEY-----`

func TestCognitoAWSJSONOperationsUseScenarioResponses(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "cognito.yml", `
name: cognito
responses:
  - { service: cognito-idp, operation: AdminGetUser, response: { Username: fixture-user, Enabled: true, UserStatus: CONFIRMED } }
  - { service: cognito-idp, operation: AdminCreateUser, response: { User: { Username: fixture-user, Enabled: true, UserStatus: CONFIRMED } } }
  - { service: cognito-idp, operation: AdminUpdateUserAttributes, response: {} }
  - { service: cognito-idp, operation: ListUsers, response: { Users: [{ Username: fixture-user, Enabled: true, UserStatus: CONFIRMED }] } }
  - { service: cognito-idp, operation: InitiateAuth, response: { AuthenticationResult: { AccessToken: raw-access-token, IdToken: raw-id-token } } }
  - { service: cognito-idp, operation: RespondToAuthChallenge, response: { AuthenticationResult: { AccessToken: raw-challenge-token } } }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios, DefaultsRoot: oidcDefaults(t)})
	session := createSession(t, handler)
	loadScenario(t, handler, session, "/scenarios/cognito.yml")

	for _, operation := range []string{"AdminGetUser", "AdminCreateUser", "AdminUpdateUserAttributes", "ListUsers", "InitiateAuth", "RespondToAuthChallenge"} {
		response := cognitoRequest(handler, session, operation, `{"ClientId":"client-1","UserPoolId":"pool"}`)
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/x-amz-json-1.0" {
			t.Fatalf("%s = (%d, %#v, %s), want aws-json-1.0 success", operation, response.Code, response.Header(), response.Body.String())
		}
		if operation == "InitiateAuth" && (!bytes.Contains(response.Body.Bytes(), []byte(`"AccessToken":"raw-access-token"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"IdToken":"raw-id-token"`))) {
			t.Fatalf("fixed raw Cognito tokens = %s", response.Body.String())
		}
	}
}

func TestCognitoJWTTemplateProducesVerifiableSessionIssuerTokens(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "jwt.yml", `
name: jwt
oidc:
  user_pool_id: ap-northeast-1_test
  signing_key:
    kid: test-rs256
    private_key_pem: |
`+indentYAML(cognitoTestPrivateKey, 6)+`
responses:
  - service: cognito-idp
    operation: InitiateAuth
    response:
      AuthenticationResult:
        AccessToken: raw-access-token
        IdToken:
          jwt:
            expires_in_seconds: 3600
            claims: { sub: alice, email: alice@example.test }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios, DefaultsRoot: oidcDefaults(t)})
	session, issuer := createOIDCSession(t, handler, "fixture.test:4566")
	if issuer != "http://fixture.test:4566/__fixture/sessions/"+session+"/oidc/ap-northeast-1_test" {
		t.Fatalf("session Host-derived issuer = %q", issuer)
	}
	loadScenarioAndRequireIssuer(t, handler, session, "/scenarios/jwt.yml", issuer)

	response := cognitoRequest(handler, session, "InitiateAuth", `{"ClientId":"client-1"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("InitiateAuth = (%d, %s)", response.Code, response.Body.String())
	}
	var result struct {
		AuthenticationResult struct{ AccessToken, IdToken string }
	}
	decodeJSON(t, response.Body, &result)
	if result.AuthenticationResult.AccessToken != "raw-access-token" {
		t.Fatalf("raw AccessToken = %q", result.AuthenticationResult.AccessToken)
	}

	discoveryResponse := httptest.NewRecorder()
	handler.ServeHTTP(discoveryResponse, httptest.NewRequest(http.MethodGet, issuer+"/.well-known/openid-configuration", nil))
	var discovery map[string]any
	decodeJSON(t, discoveryResponse.Body, &discovery)
	if discoveryResponse.Code != http.StatusOK || discovery["issuer"] != issuer || discovery["jwks_uri"] != issuer+"/.well-known/jwks.json" || discovery["authorization_endpoint"] != nil || discovery["token_endpoint"] != nil {
		t.Fatalf("OIDC discovery = (%d, %#v)", discoveryResponse.Code, discovery)
	}

	key := oidcPublicKey(t, handler, issuer)
	header, claims := verifyRS256JWT(t, result.AuthenticationResult.IdToken, key)
	if header["alg"] != "RS256" || header["kid"] != "test-rs256" || claims["iss"] != issuer || claims["sub"] != "alice" || claims["email"] != "alice@example.test" || claims["token_use"] != "id" || claims["aud"] != "client-1" {
		t.Fatalf("JWT = header %#v, claims %#v", header, claims)
	}
	if _, hasClientID := claims["client_id"]; hasClientID {
		t.Fatalf("ID token must not contain client_id: %#v", claims)
	}
	exp, iat := claims["exp"].(float64), claims["iat"].(float64)
	if exp <= iat || time.Unix(int64(exp), 0).Before(time.Now()) || time.Since(time.Unix(int64(iat), 0)) > 5*time.Second || time.Until(time.Unix(int64(iat), 0)) > 5*time.Second {
		t.Fatalf("JWT expiry = (%v, %v)", iat, exp)
	}
}

func TestCognitoJWTTemplateSetsAccessTokenUseAndAllowsExpiredTokens(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "expired.yml", `
name: expired
oidc:
  user_pool_id: ap-northeast-1_test
  signing_key:
    kid: test-rs256
    private_key_pem: |
`+indentYAML(cognitoTestPrivateKey, 6)+`
responses:
  - service: cognito-idp
    operation: RespondToAuthChallenge
    response:
      AuthenticationResult:
        AccessToken:
          jwt:
            expires_in_seconds: -1
            claims: { sub: alice }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios, DefaultsRoot: oidcDefaults(t)})
	session, issuer := createOIDCSession(t, handler, "fixture.test:4566")
	loadScenarioAndRequireIssuer(t, handler, session, "/scenarios/expired.yml", issuer)
	response := cognitoRequest(handler, session, "RespondToAuthChallenge", `{"ClientId":"client-1"}`)
	var result struct{ AuthenticationResult struct{ AccessToken string } }
	decodeJSON(t, response.Body, &result)
	_, claims := verifyRS256JWT(t, result.AuthenticationResult.AccessToken, oidcPublicKey(t, handler, issuer))
	if claims["token_use"] != "access" || claims["client_id"] != "client-1" || time.Unix(int64(claims["exp"].(float64)), 0).After(time.Now()) {
		t.Fatalf("expired access token claims = %#v", claims)
	}
}

func TestDefaultScenariosAndUseDefaultsFallback(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "strict.yml", `
name: strict
responses:
  - service: cognito-idp
    operation: AdminGetUser
    match: { Username: custom-user }
    response: { Username: custom-user }
`)
	writeScenario(t, scenarios, "fallback.yml", `
name: fallback
use_defaults: true
responses:
  - service: cognito-idp
    operation: AdminGetUser
    match: { Username: custom-user }
    response: { Username: custom-user }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios, DefaultsRoot: "defaults"})

	noScenario := createSession(t, handler)
	if got := cognitoRequest(handler, noScenario, "InitiateAuth", `{"ClientId":"client-1"}`); got.Code != http.StatusOK {
		t.Fatalf("default Cognito scenario = (%d, %s)", got.Code, got.Body.String())
	}
	if got := s3Request(handler, noScenario, http.MethodPut, "default.txt", strings.NewReader("body")); got.Code != http.StatusOK {
		t.Fatalf("default S3 scenario = (%d, %s)", got.Code, got.Body.String())
	}

	strict := createSession(t, handler)
	loadScenario(t, handler, strict, "/scenarios/strict.yml")
	if got := s3Request(handler, strict, http.MethodPut, "strict.txt", strings.NewReader("body")); got.Code != http.StatusInternalServerError || !bytes.Contains(got.Body.Bytes(), []byte("UNEXPECTED_AWS_REQUEST")) {
		t.Fatalf("strict scenario must fail fast = (%d, %s)", got.Code, got.Body.String())
	}

	fallback := createSession(t, handler)
	loadScenario(t, handler, fallback, "/scenarios/fallback.yml")
	if got := s3Request(handler, fallback, http.MethodPut, "fallback.txt", strings.NewReader("body")); got.Code != http.StatusOK {
		t.Fatalf("use_defaults fallback = (%d, %s)", got.Code, got.Body.String())
	}
	if got := cognitoRequest(handler, fallback, "AdminGetUser", `{"Username":"custom-user"}`); got.Code != http.StatusOK || !bytes.Contains(got.Body.Bytes(), []byte("custom-user")) {
		t.Fatalf("normal scenario result = (%d, %s)", got.Code, got.Body.String())
	}
	if got := cognitoRequest(handler, fallback, "AdminGetUser", `{"Username":"other-user"}`); got.Code != http.StatusInternalServerError || !bytes.Contains(got.Body.Bytes(), []byte("UNEXPECTED_AWS_REQUEST")) {
		t.Fatalf("same service + operation mismatch must remain strict = (%d, %s)", got.Code, got.Body.String())
	}
}

func cognitoRequest(handler http.Handler, session, operation, body string) *httptest.ResponseRecorder {
	request := jsonRequest(http.MethodPost, sessionAWSPath(session, "/"), body, "application/x-amz-json-1.0", "AWSCognitoIdentityProviderService."+operation)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func createOIDCSession(t *testing.T, handler http.Handler, host string) (string, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/__fixture/sessions", nil)
	request.Host = host
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var payload struct {
		SessionID string `json:"sessionId"`
		Issuer    string `json:"issuer"`
	}
	decodeJSON(t, response.Body, &payload)
	if response.Code != http.StatusCreated || payload.SessionID == "" || payload.Issuer == "" {
		t.Fatalf("create session with issuer = (%d, %#v)", response.Code, payload)
	}
	return payload.SessionID, payload.Issuer
}

func loadScenarioAndRequireIssuer(t *testing.T, handler http.Handler, session, path, wantIssuer string) {
	t.Helper()
	response := serveJSON(handler, http.MethodPost, sessionControlPath(session, "scenario"), map[string]string{"path": path})
	var payload struct {
		Loaded bool   `json:"loaded"`
		Issuer string `json:"issuer"`
	}
	decodeJSON(t, response.Body, &payload)
	if response.Code != http.StatusOK || !payload.Loaded || payload.Issuer != wantIssuer {
		t.Fatalf("load scenario issuer = (%d, %#v)", response.Code, payload)
	}
}

func oidcPublicKey(t *testing.T, handler http.Handler, issuer string) *rsa.PublicKey {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, issuer+"/.well-known/jwks.json", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("JWKS = (%d, %s)", response.Code, response.Body.String())
	}
	var jwks struct {
		Keys []struct{ Kty, Kid, Use, Alg, N, E string }
	}
	decodeJSON(t, response.Body, &jwks)
	if len(jwks.Keys) != 1 || jwks.Keys[0].Kty != "RSA" || jwks.Keys[0].Kid != "test-rs256" || jwks.Keys[0].Use != "sig" || jwks.Keys[0].Alg != "RS256" {
		t.Fatalf("JWKS = %#v", jwks)
	}
	n, err := base64.RawURLEncoding.DecodeString(jwks.Keys[0].N)
	if err != nil {
		t.Fatal(err)
	}
	e, err := base64.RawURLEncoding.DecodeString(jwks.Keys[0].E)
	if err != nil {
		t.Fatal(err)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
}

func verifyRS256JWT(t *testing.T, token string, key *rsa.PublicKey) (map[string]any, map[string]any) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT has %d parts: %q", len(parts), token)
	}
	decode := func(part string) map[string]any {
		data, err := base64.RawURLEncoding.DecodeString(part)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	header, claims := decode(parts[0]), decode(parts[1])
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatalf("JWT signature: %v", err)
	}
	return header, claims
}

func indentYAML(value string, spaces int) string {
	return strings.Repeat(" ", spaces) + strings.ReplaceAll(value, "\n", "\n"+strings.Repeat(" ", spaces))
}

func TestCognitoReservedJWTClaimsAreRejected(t *testing.T) {
	scenarios := t.TempDir()
	writeScenario(t, scenarios, "reserved.yml", `
name: reserved
responses:
  - service: cognito-idp
    operation: InitiateAuth
    response:
      AuthenticationResult:
        AccessToken:
          jwt:
            expires_in_seconds: 3600
            claims: { sub: alice, token_use: id }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios})
	session := createSession(t, handler)
	response := serveJSON(handler, http.MethodPost, sessionControlPath(session, "scenario"), map[string]string{"path": "/scenarios/reserved.yml"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("reserved JWT claim load = (%d, %s)", response.Code, response.Body.String())
	}
}

func TestCognitoFixtureTestKeyIsRSA2048(t *testing.T) {
	block, _ := pem.Decode([]byte(cognitoTestPrivateKey))
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if key.(*rsa.PrivateKey).N.BitLen() != 2048 {
		t.Fatal("test key must be RSA 2048")
	}
}

func TestOIDCRoutesDistinguishMissingConfigurationAndUnknownSessions(t *testing.T) {
	handler := mustNewHandler(t, Config{ScenarioRoot: t.TempDir()})
	handler.(*fixture).oidc = nil
	session := createSession(t, handler)
	for _, suffix := range []string{"/.well-known/openid-configuration", "/.well-known/jwks.json"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, sessionControlPath(session, "oidc/ap-northeast-1_test")+suffix, nil))
		assertOIDCError(t, response, http.StatusConflict, "OIDC_NOT_CONFIGURED")
	}
	unknown := httptest.NewRecorder()
	handler.ServeHTTP(unknown, httptest.NewRequest(http.MethodGet, "/__fixture/sessions/missing/oidc/ap-northeast-1_test/.well-known/jwks.json", nil))
	assertOIDCError(t, unknown, http.StatusNotFound, "NOT_FOUND")
}

func TestNewHandlerUsesBundledDefaultsWhenDefaultsRootIsOmitted(t *testing.T) {
	handler := mustNewHandler(t, Config{ScenarioRoot: t.TempDir()})
	session := createSession(t, handler)
	if response := cognitoRequest(handler, session, "AdminGetUser", `{"UserPoolId":"pool","Username":"fixture-user"}`); response.Code != http.StatusOK {
		t.Fatalf("omitted DefaultsRoot must use bundled defaults = (%d, %s)", response.Code, response.Body.String())
	}
}

func TestScenarioOIDCUserPoolOverrideChangesReturnedIssuer(t *testing.T) {
	scenarios, defaults := t.TempDir(), oidcDefaults(t)
	writeScenario(t, scenarios, "override.yml", `
name: override
oidc:
  user_pool_id: ap-northeast-1_override
responses: []
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios, DefaultsRoot: defaults})
	session, issuer := createOIDCSession(t, handler, "fixture.test:4566")
	if !strings.HasSuffix(issuer, "/oidc/ap-northeast-1_test") {
		t.Fatalf("default issuer = %q", issuer)
	}
	response := serveJSON(handler, http.MethodPost, sessionControlPath(session, "scenario"), map[string]string{"path": "/scenarios/override.yml"})
	var result struct {
		Issuer string `json:"issuer"`
	}
	decodeJSON(t, response.Body, &result)
	if response.Code != http.StatusOK || result.Issuer != strings.TrimSuffix(issuer, "ap-northeast-1_test")+"ap-northeast-1_override" {
		t.Fatalf("overridden issuer = (%d, %#v)", response.Code, result)
	}
}

func TestJWTTemplateUsesDefaultOIDCConfigAndMergesPartialScenarioOverride(t *testing.T) {
	defaults, scenarios := copyBundledDefaults(t), t.TempDir()
	writeDefaultsConfig(t, defaults, cognitoDefaultsConfig(cognitoTestPrivateKey))
	writeScenario(t, scenarios, "base.yml", `
name: base
responses:
  - service: cognito-idp
    operation: InitiateAuth
    response:
      AuthenticationResult:
        AccessToken: { jwt: { expires_in_seconds: 60, claims: { sub: base-user } } }
`)
	writeScenario(t, scenarios, "partial.yml", `
name: partial
oidc:
  user_pool_id: ap-northeast-1_partial
responses:
  - service: cognito-idp
    operation: InitiateAuth
    response:
      AuthenticationResult:
        AccessToken: { jwt: { expires_in_seconds: 60, claims: { sub: partial-user } } }
`)
	handler, err := NewHandler(Config{ScenarioRoot: scenarios, DefaultsRoot: defaults})
	if err != nil {
		t.Fatalf("valid defaults startup: %v", err)
	}
	baseSession, baseIssuer := createOIDCSession(t, handler, "fixture.test:4566")
	loadScenarioAndRequireIssuer(t, handler, baseSession, "/scenarios/base.yml", baseIssuer)
	baseResponse := cognitoRequest(handler, baseSession, "InitiateAuth", `{"ClientId":"client-1"}`)
	var baseResult struct{ AuthenticationResult struct{ AccessToken string } }
	decodeJSON(t, baseResponse.Body, &baseResult)
	_, baseClaims := verifyRS256JWT(t, baseResult.AuthenticationResult.AccessToken, oidcPublicKeyWithKid(t, handler, baseIssuer, "fixture-default-rs256"))
	if baseClaims["sub"] != "base-user" || baseClaims["iss"] != baseIssuer {
		t.Fatalf("default OIDC config JWT = %#v", baseClaims)
	}
	partialSession, _ := createOIDCSession(t, handler, "fixture.test:4566")
	partialIssuer := "http://fixture.test:4566/__fixture/sessions/" + partialSession + "/oidc/ap-northeast-1_partial"
	loadScenarioAndRequireIssuer(t, handler, partialSession, "/scenarios/partial.yml", partialIssuer)
	partialResponse := cognitoRequest(handler, partialSession, "InitiateAuth", `{"ClientId":"client-1"}`)
	var partialResult struct{ AuthenticationResult struct{ AccessToken string } }
	decodeJSON(t, partialResponse.Body, &partialResult)
	_, partialClaims := verifyRS256JWT(t, partialResult.AuthenticationResult.AccessToken, oidcPublicKeyWithKid(t, handler, partialIssuer, "fixture-default-rs256"))
	if partialClaims["sub"] != "partial-user" || partialClaims["iss"] != partialIssuer {
		t.Fatalf("partial OIDC override JWT = %#v", partialClaims)
	}
}

func TestInvalidJWTTemplatesAreRejectedAtScenarioLoad(t *testing.T) {
	for name, response := range map[string]string{
		"missing-sub":         `AccessToken: { jwt: { expires_in_seconds: 1, claims: { email: test@example.test } } }`,
		"missing-expiry":      `AccessToken: { jwt: { claims: { sub: alice } } }`,
		"reserved-iss":        `AccessToken: { jwt: { expires_in_seconds: 1, claims: { sub: alice, iss: invalid } } }`,
		"reserved-iat":        `AccessToken: { jwt: { expires_in_seconds: 1, claims: { sub: alice, iat: 1 } } }`,
		"reserved-exp":        `AccessToken: { jwt: { expires_in_seconds: 1, claims: { sub: alice, exp: 1 } } }`,
		"reserved-token-use":  `AccessToken: { jwt: { expires_in_seconds: 1, claims: { sub: alice, token_use: id } } }`,
		"reserved-client-id":  `AccessToken: { jwt: { expires_in_seconds: 1, claims: { sub: alice, client_id: client } } }`,
		"reserved-aud":        `AccessToken: { jwt: { expires_in_seconds: 1, claims: { sub: alice, aud: client } } }`,
		"outside-auth-result": `jwt: { expires_in_seconds: 1, claims: { sub: alice } }`,
		"InitiateAuth outside AuthenticationResult": `Other: { jwt: { expires_in_seconds: 1, claims: { sub: alice } } }`,
	} {
		t.Run(name, func(t *testing.T) {
			scenarios := t.TempDir()
			operation, body := "InitiateAuth", "AuthenticationResult:\n        "+response
			if name == "outside-auth-result" {
				operation, body = "AdminGetUser", response
			}
			writeScenario(t, scenarios, "invalid.yml", "name: invalid\nresponses:\n  - service: cognito-idp\n    operation: "+operation+"\n    response:\n      "+body+"\n")
			handler := mustNewHandler(t, Config{ScenarioRoot: scenarios, DefaultsRoot: oidcDefaults(t)})
			session := createSession(t, handler)
			loaded := serveJSON(handler, http.MethodPost, sessionControlPath(session, "scenario"), map[string]string{"path": "/scenarios/invalid.yml"})
			if loaded.Code != http.StatusBadRequest {
				t.Fatalf("%s = (%d, %s)", name, loaded.Code, loaded.Body.String())
			}
		})
	}
}

func TestJWTTemplateWithoutFinalSigningKeyIsRejected(t *testing.T) {
	scenarios, defaults := t.TempDir(), copyBundledDefaults(t)
	writeDefaultsConfig(t, defaults, `service: cognito-idp
protocol: aws-json-1.0
oidc:
  user_pool_id: ap-northeast-1_test
`)
	writeScenario(t, scenarios, "no-key.yml", `
name: no-key
responses:
  - service: cognito-idp
    operation: InitiateAuth
    response:
      AuthenticationResult:
        AccessToken: { jwt: { expires_in_seconds: 1, claims: { sub: alice } } }
`)
	if _, err := NewHandler(Config{ScenarioRoot: scenarios, DefaultsRoot: defaults}); err == nil {
		t.Fatal("missing default signing key must fail startup")
	}
}

func TestExplicitIncompatibleJWKSIsReturnedWithoutScenarioLoadValidation(t *testing.T) {
	scenarios := t.TempDir()
	other := mustRSAJWK(t)
	writeScenario(t, scenarios, "incompatible-jwks.yml", `
name: incompatible-jwks
oidc:
  user_pool_id: ap-northeast-1_test
  signing_key:
    kid: signing-key
    private_key_pem: |
`+indentYAML(cognitoTestPrivateKey, 6)+`
  jwks:
    keys:
      - `+other+`
responses:
  - service: cognito-idp
    operation: InitiateAuth
    response:
      AuthenticationResult:
        AccessToken: { jwt: { expires_in_seconds: 60, claims: { sub: alice } } }
`)
	handler := mustNewHandler(t, Config{ScenarioRoot: scenarios, DefaultsRoot: oidcDefaults(t)})
	session, issuer := createOIDCSession(t, handler, "fixture.test:4566")
	loadScenarioAndRequireIssuer(t, handler, session, "/scenarios/incompatible-jwks.yml", issuer)
	response := cognitoRequest(handler, session, "InitiateAuth", `{"ClientId":"client-1"}`)
	var result struct{ AuthenticationResult struct{ AccessToken string } }
	decodeJSON(t, response.Body, &result)
	jwksResponse := httptest.NewRecorder()
	handler.ServeHTTP(jwksResponse, httptest.NewRequest(http.MethodGet, issuer+"/.well-known/jwks.json", nil))
	var jwks struct {
		Keys []struct{ Kty, Kid, Use, Alg, N, E string }
	}
	decodeJSON(t, jwksResponse.Body, &jwks)
	wantN := strings.Split(strings.Split(other, "\n")[4], ": ")[1]
	if jwksResponse.Code != http.StatusOK || len(jwks.Keys) != 1 || jwks.Keys[0].Kty != "RSA" || jwks.Keys[0].Kid != "incompatible-key" || jwks.Keys[0].Use != "sig" || jwks.Keys[0].Alg != "RS256" || jwks.Keys[0].N != wantN || jwks.Keys[0].E != "AQAB" {
		t.Fatalf("explicit JWKS must be returned unchanged: (%d, %#v)", jwksResponse.Code, jwks)
	}
	key := oidcPublicKeyWithKid(t, handler, issuer, "incompatible-key")
	parts := strings.Split(result.AuthenticationResult.AccessToken, ".")
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err == nil {
		t.Fatal("incompatible JWKS must not verify the JWT signature")
	}
}

func TestDefaultScenarioCoversAllSupportedOperations(t *testing.T) {
	handler := mustNewHandler(t, Config{ScenarioRoot: t.TempDir(), DefaultsRoot: "defaults"})
	session := createSession(t, handler)
	for _, operation := range []string{"AdminGetUser", "AdminCreateUser", "AdminUpdateUserAttributes", "ListUsers", "InitiateAuth", "RespondToAuthChallenge"} {
		response := cognitoRequest(handler, session, operation, `{"ClientId":"client-1"}`)
		if response.Code != http.StatusOK {
			t.Fatalf("default %s = (%d, %s)", operation, response.Code, response.Body.String())
		}
		if operation == "AdminGetUser" && (!bytes.Contains(response.Body.Bytes(), []byte(`"Username":"fixture-user"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"UserStatus":"CONFIRMED"`))) {
			t.Fatalf("default user = %s", response.Body.String())
		}
		if (operation == "AdminCreateUser" || operation == "ListUsers") && !bytes.Contains(response.Body.Bytes(), []byte(`"Username":"fixture-user"`)) {
			t.Fatalf("default %s user identity = %s", operation, response.Body.String())
		}
		if operation == "InitiateAuth" && (!bytes.Contains(response.Body.Bytes(), []byte(`"TokenType":"Bearer"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"ExpiresIn":3600`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"RefreshToken":"refresh-token-will-accept-any-value"`))) {
			t.Fatalf("default auth = %s", response.Body.String())
		}
		if operation == "RespondToAuthChallenge" && (!bytes.Contains(response.Body.Bytes(), []byte(`"AuthenticationResult"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"AccessToken"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"IdToken"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"RefreshToken":"refresh-token-will-accept-any-value"`))) {
			t.Fatalf("default challenge auth = %s", response.Body.String())
		}
	}
	for _, response := range []*httptest.ResponseRecorder{
		s3Request(handler, session, http.MethodGet, "object.txt", nil), s3Request(handler, session, http.MethodPut, "object.txt", strings.NewReader("body")), s3Request(handler, session, http.MethodHead, "object.txt", nil),
		secret(t, handler, session, "anything"), sendMessage(handler, session, "https://sqs.us-east-1.amazonaws.com/123/queue", "hello"),
	} {
		if response.Code != http.StatusOK {
			t.Fatalf("default supported operation = (%d, %s)", response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"/model/default/invoke", "/model/default/converse"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, jsonRequest(http.MethodPost, sessionAWSPath(session, path), `{}`, "application/json", ""))
		if response.Code != http.StatusOK {
			t.Fatalf("default Bedrock %s = (%d, %s)", path, response.Code, response.Body.String())
		}
	}
}

func oidcDefaults(t *testing.T) string {
	t.Helper()
	root := copyBundledDefaults(t)
	writeDefaultsConfig(t, root, `service: cognito-idp
protocol: aws-json-1.0
oidc:
  user_pool_id: ap-northeast-1_test
  signing_key:
    kid: test-rs256
    private_key_pem: |
`+indentYAML(cognitoTestPrivateKey, 6))
	return root
}
func writeDefaultsConfig(t *testing.T, root, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "cognito-idp.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
func assertOIDCError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var body map[string]any
	decodeJSON(t, response.Body, &body)
	if response.Code != status || body["error"] != code {
		t.Fatalf("OIDC error = (%d, %#v), want (%d, %s)", response.Code, body, status, code)
	}
}
func mustRSAJWK(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("kty: RSA\n        kid: incompatible-key\n        use: sig\n        alg: RS256\n        n: %s\n        e: AQAB", base64.RawURLEncoding.EncodeToString(key.N.Bytes()))
}
func oidcPublicKeyWithKid(t *testing.T, handler http.Handler, issuer, wantKid string) *rsa.PublicKey {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, issuer+"/.well-known/jwks.json", nil))
	var jwks struct{ Keys []struct{ Kid, N, E string } }
	decodeJSON(t, response.Body, &jwks)
	for _, jwk := range jwks.Keys {
		if jwk.Kid == wantKid {
			n, _ := base64.RawURLEncoding.DecodeString(jwk.N)
			e, _ := base64.RawURLEncoding.DecodeString(jwk.E)
			return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
		}
	}
	t.Fatalf("missing JWKS kid %q: %#v", wantKid, jwks)
	return nil
}

func TestNewHandlerRejectsInvalidBundledDefaultsAtStartup(t *testing.T) {
	for name, corrupt := range map[string]func(t *testing.T, root string){
		"malformed YAML": func(t *testing.T, root string) {
			writeTestFile(t, filepath.Join(root, "config", "cognito-idp.yml"), ": not valid YAML")
		},
		"unknown schema field": func(t *testing.T, root string) {
			appendTestFile(t, filepath.Join(root, "config", "cognito-idp.yml"), "\nunknown: value\n")
		},
		"invalid signing key": func(t *testing.T, root string) {
			writeTestFile(t, filepath.Join(root, "config", "cognito-idp.yml"), cognitoDefaultsConfig("not a PEM"))
		},
		"non-RSA signing key": func(t *testing.T, root string) {
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			der, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, filepath.Join(root, "config", "cognito-idp.yml"), cognitoDefaultsConfig(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))))
		},
		"non-2048 RSA signing key": func(t *testing.T, root string) {
			key, err := rsa.GenerateKey(rand.Reader, 1024)
			if err != nil {
				t.Fatal(err)
			}
			der, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, filepath.Join(root, "config", "cognito-idp.yml"), cognitoDefaultsConfig(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))))
		},
		"invalid default JWT template": func(t *testing.T, root string) {
			writeTestFile(t, filepath.Join(root, "scenario", "cognito-idp.yml"), `name: default-cognito
responses:
  - service: cognito-idp
    operation: InitiateAuth
    response:
      AuthenticationResult:
        AccessToken: { jwt: { expires_in_seconds: 60, claims: {} } }
`)
		},
		"missing required defaults file": func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "config", "cognito-idp.yml")); err != nil {
				t.Fatal(err)
			}
		},
		"duplicate default service operation": func(t *testing.T, root string) {
			appendTestFile(t, filepath.Join(root, "scenario", "s3.yml"), `
  - service: s3
    operation: PutObject
    response: { ETag: '"duplicate"' }
`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := copyBundledDefaults(t)
			corrupt(t, root)
			_, err := NewHandler(Config{ScenarioRoot: t.TempDir(), DefaultsRoot: root})
			if err == nil {
				t.Fatal("NewHandler must reject invalid bundled defaults at startup")
			}
		})
	}
}

func copyBundledDefaults(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	copyDefaultsDirectory(t, filepath.Join("..", "..", "defaults"), root)
	return root
}

func copyDefaultsDirectory(t *testing.T, source, destination string) {
	t.Helper()
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		sourcePath, destinationPath := filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())
		if entry.IsDir() {
			copyDefaultsDirectory(t, sourcePath, destinationPath)
			continue
		}
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destinationPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func cognitoDefaultsConfig(key string) string {
	return "service: cognito-idp\nprotocol: aws-json-1.0\noidc:\n  user_pool_id: ap-northeast-1_test\n  signing_key:\n    kid: fixture-default-rs256\n    private_key_pem: |\n" + indentYAML(key, 6) + "\n"
}
func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
func appendTestFile(t *testing.T, path, body string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(body); err != nil {
		t.Fatal(err)
	}
}
