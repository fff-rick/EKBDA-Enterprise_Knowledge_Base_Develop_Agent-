package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type testJWKS struct {
	mu   sync.RWMutex
	keys map[string]*rsa.PublicKey
}

func (j *testJWKS) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	keys := make([]map[string]string, 0, len(j.keys))
	for keyID, key := range j.keys {
		keys = append(keys, map[string]string{
			"kty": "RSA", "kid": keyID, "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		})
	}
	_ = json.NewEncoder(response).Encode(map[string]any{"keys": keys})
}

func (j *testJWKS) replace(keyID string, key *rsa.PublicKey) {
	j.mu.Lock()
	j.keys = map[string]*rsa.PublicKey{keyID: key}
	j.mu.Unlock()
}

func newTestJWTAuthenticator(t *testing.T) (*JWTAuthenticator, *testJWKS, *rsa.PrivateKey, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	keys := &testJWKS{keys: map[string]*rsa.PublicKey{"key-1": &privateKey.PublicKey}}
	server := httptest.NewServer(keys)
	t.Cleanup(server.Close)
	authenticator, err := NewJWTAuthenticator(context.Background(), JWTConfig{
		Issuer: "https://sso.example", Audience: "ekbda-api", JWKSURL: server.URL,
		RolesClaim: "realm_access.roles", AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("create JWT authenticator: %v", err)
	}
	return authenticator, keys, privateKey, server.URL
}

func signedToken(t *testing.T, key *rsa.PrivateKey, keyID string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	encoded, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return encoded
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":          "https://sso.example",
		"aud":          []string{"ekbda-api"},
		"sub":          "employee-123",
		"iat":          time.Now().Add(-time.Minute).Unix(),
		"exp":          time.Now().Add(time.Hour).Unix(),
		"realm_access": map[string]any{"roles": []any{"Developer", "knowledge_admin", "Developer"}},
	}
}

func TestJWTAuthenticatorReturnsVerifiedIdentity(t *testing.T) {
	authenticator, _, privateKey, _ := newTestJWTAuthenticator(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/search", nil)
	request.Header.Set("Authorization", "Bearer "+signedToken(t, privateKey, "key-1", validClaims()))
	request.Header.Set("X-User-ID", "spoofed-user")
	request.Header.Set("X-User-Roles", "spoofed-role")
	identity, err := authenticator.Authenticate(request)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if identity.UserID != "employee-123" || identity.Source != "jwt" || len(identity.Roles) != 2 || identity.Roles[0] != "developer" || identity.Roles[1] != "knowledge_admin" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
}

func TestJWTAuthenticatorRejectsInvalidClaimsAndAlgorithm(t *testing.T) {
	authenticator, _, privateKey, _ := newTestJWTAuthenticator(t)
	tests := []struct {
		name  string
		token func(*testing.T) string
	}{
		{name: "expired", token: func(t *testing.T) string {
			claims := validClaims()
			claims["exp"] = time.Now().Add(-time.Hour).Unix()
			return signedToken(t, privateKey, "key-1", claims)
		}},
		{name: "wrong audience", token: func(t *testing.T) string {
			claims := validClaims()
			claims["aud"] = "another-api"
			return signedToken(t, privateKey, "key-1", claims)
		}},
		{name: "missing expiration", token: func(t *testing.T) string {
			claims := validClaims()
			delete(claims, "exp")
			return signedToken(t, privateKey, "key-1", claims)
		}},
		{name: "HMAC algorithm", token: func(t *testing.T) string {
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims())
			token.Header["kid"] = "key-1"
			encoded, err := token.SignedString([]byte("not-an-rsa-key"))
			if err != nil {
				t.Fatalf("sign HMAC JWT: %v", err)
			}
			return encoded
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", "Bearer "+test.token(t))
			if _, err := authenticator.Authenticate(request); err == nil {
				t.Fatal("expected authentication failure")
			}
		})
	}
}

func TestJWTAuthenticatorRefreshesUnknownKeyID(t *testing.T) {
	authenticator, keys, _, _ := newTestJWTAuthenticator(t)
	rotatedKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rotated RSA key: %v", err)
	}
	keys.replace("key-2", &rotatedKey.PublicKey)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+signedToken(t, rotatedKey, "key-2", validClaims()))
	identity, err := authenticator.Authenticate(request)
	if err != nil || identity.UserID != "employee-123" {
		t.Fatalf("authenticate with rotated key: identity=%#v err=%v", identity, err)
	}
}

func TestJWTAuthenticatorRequiresHTTPSJWKS(t *testing.T) {
	_, err := NewJWTAuthenticator(context.Background(), JWTConfig{
		Issuer: "https://sso.example", Audience: "ekbda-api", JWKSURL: "http://sso.example/jwks",
	})
	if err == nil {
		t.Fatal("expected insecure JWKS URL to be rejected")
	}
}

func TestJWTAuthenticatorRejectsHTTPSRedirectDowngrade(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(response).Encode(map[string]any{"keys": []any{}})
	}))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, target.URL, http.StatusFound)
	}))
	defer source.Close()
	_, err := NewJWTAuthenticator(context.Background(), JWTConfig{
		Issuer: "https://sso.example", Audience: "ekbda-api", JWKSURL: source.URL, HTTPClient: source.Client(),
	})
	if err == nil {
		t.Fatal("expected HTTPS to HTTP JWKS redirect to be rejected")
	}
}
