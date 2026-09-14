package server

import (
	"crypto"
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
	"time"
)

type oidcConfig struct {
	UserPoolID string                 `yaml:"user_pool_id"`
	SigningKey *signingKey            `yaml:"signing_key"`
	JWKS       map[string]interface{} `yaml:"jwks"`
}

type signingKey struct {
	Kid           string `yaml:"kid"`
	PrivateKeyPEM string `yaml:"private_key_pem"`
	key           *rsa.PrivateKey
}

func mergeOIDC(base, override *oidcConfig) *oidcConfig {
	if base == nil && override == nil {
		return nil
	}
	var result oidcConfig
	if base != nil {
		result = *base
	}
	if override == nil {
		return &result
	}
	if override.UserPoolID != "" {
		result.UserPoolID = override.UserPoolID
	}
	if override.SigningKey != nil {
		result.SigningKey = override.SigningKey
	}
	if override.JWKS != nil {
		result.JWKS = override.JWKS
	}
	return &result
}

func validateOIDC(config *oidcConfig, requireKey bool) error {
	if config == nil {
		return nil
	}
	if config.UserPoolID == "" {
		return fmt.Errorf("oidc.user_pool_id is required")
	}
	if config.SigningKey == nil {
		if requireKey {
			return fmt.Errorf("oidc.signing_key is required")
		}
		return nil
	}
	if config.SigningKey.Kid == "" || config.SigningKey.PrivateKeyPEM == "" {
		return fmt.Errorf("oidc signing key kid and private_key_pem are required")
	}
	block, _ := pem.Decode([]byte(config.SigningKey.PrivateKeyPEM))
	if block == nil {
		return fmt.Errorf("invalid oidc private key PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse oidc private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok || key.N.BitLen() != 2048 {
		return fmt.Errorf("oidc signing key must be RSA 2048")
	}
	config.SigningKey.key = key
	return nil
}

func (f *fixture) issuer(s *session, oidc *oidcConfig) string {
	if oidc == nil || oidc.UserPoolID == "" {
		return ""
	}
	return "http://" + s.host + "/__fixture/sessions/" + s.id + "/oidc/" + oidc.UserPoolID
}

func (f *fixture) sessionOIDC(s *session) *oidcConfig {
	if s.scenario == nil {
		return mergeOIDC(f.oidc, nil)
	}
	return mergeOIDC(f.oidc, s.scenario.OIDC)
}

func (f *fixture) oidcRoute(w http.ResponseWriter, r *http.Request, id string, tail []string) bool {
	if len(tail) != 4 || tail[0] != "oidc" || tail[2] != ".well-known" || (tail[3] != "openid-configuration" && tail[3] != "jwks.json") || r.Method != http.MethodGet {
		return false
	}
	f.mu.Lock()
	s, ok := f.sessions[id]
	f.mu.Unlock()
	if !ok {
		controlError(w, http.StatusNotFound, "NOT_FOUND", "fixture session not found")
		return true
	}
	oidc := f.sessionOIDC(s)
	if oidc == nil || oidc.UserPoolID == "" {
		controlError(w, http.StatusConflict, "OIDC_NOT_CONFIGURED", "OIDC is not configured")
		return true
	}
	if tail[1] != oidc.UserPoolID {
		controlError(w, http.StatusNotFound, "NOT_FOUND", "OIDC user pool not found")
		return true
	}
	issuer := f.issuer(s, oidc)
	if tail[3] == "openid-configuration" {
		writeJSON(w, http.StatusOK, map[string]string{"issuer": issuer, "jwks_uri": issuer + "/.well-known/jwks.json"})
		return true
	}
	if oidc.JWKS != nil {
		writeJSON(w, http.StatusOK, oidc.JWKS)
		return true
	}
	if oidc.SigningKey == nil || oidc.SigningKey.key == nil {
		controlError(w, http.StatusConflict, "OIDC_NOT_CONFIGURED", "OIDC signing key is not configured")
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": oidc.SigningKey.Kid, "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(oidc.SigningKey.key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(oidc.SigningKey.key.E)).Bytes())}}})
	return true
}

func jwtToken(template map[string]any, oidc *oidcConfig, issuer, use, clientID string) (string, error) {
	jwt, ok := template["jwt"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("jwt template is invalid")
	}
	expires, ok := jwt["expires_in_seconds"].(int)
	if !ok {
		if number, numeric := jwt["expires_in_seconds"].(float64); numeric {
			expires = int(number)
			ok = true
		}
	}
	if !ok {
		return "", fmt.Errorf("jwt expires_in_seconds is required")
	}
	claims, ok := jwt["claims"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("jwt claims are required")
	}
	now := time.Now().Unix()
	payload := map[string]any{}
	for k, v := range claims {
		payload[k] = v
	}
	payload["iss"], payload["iat"], payload["exp"], payload["token_use"] = issuer, now, now+int64(expires), use
	if use == "access" {
		payload["client_id"] = clientID
	} else {
		payload["aud"] = clientID
	}
	header := map[string]string{"alg": "RS256", "kid": oidc.SigningKey.Kid}
	h, _ := json.Marshal(header)
	p, _ := json.Marshal(payload)
	signing := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, oidc.SigningKey.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func validateJWTTemplates(sc *scenario, oidc *oidcConfig) error {
	for _, d := range sc.Responses {
		if err := validateJWTResult(d.Service, d.Operation, d.Response); err != nil {
			return err
		}
		for _, r := range d.Sequence {
			if err := validateJWTResult(d.Service, d.Operation, r.Response); err != nil {
				return err
			}
		}
	}
	for _, d := range sc.Responses {
		if hasJWT(d.Response) || anySequenceJWT(d.Sequence) {
			if oidc == nil || oidc.SigningKey == nil || oidc.SigningKey.key == nil {
				return fmt.Errorf("JWT template requires final OIDC signing key")
			}
		}
	}
	return nil
}
func anySequenceJWT(results []result) bool {
	for _, r := range results {
		if hasJWT(r.Response) {
			return true
		}
	}
	return false
}
func hasJWT(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		if _, ok := x["jwt"]; ok {
			return true
		}
		for _, y := range x {
			if hasJWT(y) {
				return true
			}
		}
	case []any:
		for _, y := range x {
			if hasJWT(y) {
				return true
			}
		}
	}
	return false
}
func validateJWTResult(service, operation string, response map[string]any) error {
	if response == nil {
		return nil
	}
	allowed := service == "cognito-idp" && (operation == "InitiateAuth" || operation == "RespondToAuthChallenge")
	if !allowed && hasJWT(response) {
		return fmt.Errorf("jwt template is only allowed for Cognito authentication results")
	}
	auth, _ := response["AuthenticationResult"].(map[string]any)
	for _, field := range []string{"AccessToken", "IdToken"} {
		value, ok := auth[field]
		if !ok {
			continue
		}
		template, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := template["jwt"]; !ok {
			continue
		}
		if len(template) != 1 {
			return fmt.Errorf("jwt token template must contain only jwt")
		}
		jwt, ok := template["jwt"].(map[string]any)
		if !ok || len(jwt) != 2 {
			return fmt.Errorf("jwt template is invalid")
		}
		expires, ok := jwt["expires_in_seconds"].(int)
		if !ok {
			return fmt.Errorf("jwt expires_in_seconds is required")
		}
		_ = expires
		claims, ok := jwt["claims"].(map[string]any)
		if !ok {
			return fmt.Errorf("jwt claims are required")
		}
		if _, ok := claims["sub"]; !ok {
			return fmt.Errorf("jwt claims.sub is required")
		}
		for _, reserved := range []string{"iss", "iat", "exp", "token_use", "client_id", "aud"} {
			if _, ok := claims[reserved]; ok {
				return fmt.Errorf("jwt claims.%s is reserved", reserved)
			}
		}
	}
	if hasJWT(response) {
		for key, value := range response {
			if key != "AuthenticationResult" && hasJWT(value) {
				return fmt.Errorf("jwt template is only allowed in AuthenticationResult")
			}
		}
		if auth == nil {
			return fmt.Errorf("jwt template is only allowed in AuthenticationResult")
		}
		for key, value := range auth {
			if key != "AccessToken" && key != "IdToken" && hasJWT(value) {
				return fmt.Errorf("jwt template is only allowed for AccessToken or IdToken")
			}
		}
	}
	return nil
}

func cloneMap(value map[string]any) map[string]any {
	b, _ := json.Marshal(value)
	var copy map[string]any
	_ = json.Unmarshal(b, &copy)
	return copy
}

func (f *fixture) renderCognitoTokens(response map[string]any, s *session, clientID string) (map[string]any, error) {
	copy := cloneMap(response)
	auth, _ := copy["AuthenticationResult"].(map[string]any)
	if auth == nil {
		return copy, nil
	}
	oidc := f.sessionOIDC(s)
	issuer := f.issuer(s, oidc)
	for field, use := range map[string]string{"AccessToken": "access", "IdToken": "id"} {
		if template, ok := auth[field].(map[string]any); ok {
			if _, yes := template["jwt"]; yes {
				token, err := jwtToken(template, oidc, issuer, use, clientID)
				if err != nil {
					return nil, err
				}
				auth[field] = token
			}
		}
	}
	return copy, nil
}
