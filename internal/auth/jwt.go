package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
)

const (
	maxTokenBytes       = 16 << 10
	maxJWKSBytes        = 1 << 20
	unknownKeyRetryWait = 5 * time.Second
)

type JWTConfig struct {
	Issuer            string
	Audience          string
	JWKSURL           string
	UserIDClaim       string
	RolesClaim        string
	ClockSkew         time.Duration
	AllowInsecureHTTP bool
	HTTPClient        *http.Client
}

type JWTAuthenticator struct {
	config JWTConfig
	client *http.Client

	keysMu sync.RWMutex
	keys   map[string]*rsa.PublicKey

	refreshMu          sync.Mutex
	lastUnknownRefresh time.Time
}

func NewJWTAuthenticator(ctx context.Context, config JWTConfig) (*JWTAuthenticator, error) {
	config.Issuer = strings.TrimSpace(config.Issuer)
	config.Audience = strings.TrimSpace(config.Audience)
	config.JWKSURL = strings.TrimSpace(config.JWKSURL)
	config.UserIDClaim = strings.TrimSpace(config.UserIDClaim)
	config.RolesClaim = strings.TrimSpace(config.RolesClaim)
	if config.UserIDClaim == "" {
		config.UserIDClaim = "sub"
	}
	if config.RolesClaim == "" {
		config.RolesClaim = "roles"
	}
	if config.ClockSkew < 0 {
		return nil, fmt.Errorf("JWT clock skew cannot be negative")
	}
	if config.Issuer == "" || config.Audience == "" || config.JWKSURL == "" {
		return nil, fmt.Errorf("JWT issuer, audience and JWKS URL are required")
	}
	parsedURL, err := url.Parse(config.JWKSURL)
	if err != nil || parsedURL.Host == "" || parsedURL.User != nil || parsedURL.Fragment != "" {
		return nil, fmt.Errorf("JWT JWKS URL is invalid")
	}
	if parsedURL.Scheme != "https" && !(config.AllowInsecureHTTP && parsedURL.Scheme == "http") {
		return nil, fmt.Errorf("JWT JWKS URL must use HTTPS")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	clientCopy := *client
	previousRedirectPolicy := client.CheckRedirect
	clientCopy.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" && !(config.AllowInsecureHTTP && request.URL.Scheme == "http") {
			return fmt.Errorf("JWKS redirect must use HTTPS")
		}
		if previousRedirectPolicy != nil {
			return previousRedirectPolicy(request, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("too many JWKS redirects")
		}
		return nil
	}
	authenticator := &JWTAuthenticator{config: config, client: &clientCopy}
	if err := authenticator.loadKeys(ctx); err != nil {
		return nil, err
	}
	return authenticator, nil
}

func (*JWTAuthenticator) Mode() string { return "jwt" }

func (a *JWTAuthenticator) Authenticate(request *http.Request) (Identity, error) {
	rawToken, err := bearerToken(request.Header.Get("Authorization"))
	if err != nil {
		return Identity{}, err
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected JWT signing algorithm")
		}
		keyID, ok := token.Header["kid"].(string)
		if !ok || strings.TrimSpace(keyID) == "" {
			return nil, fmt.Errorf("JWT key ID is required")
		}
		return a.keyFor(request.Context(), keyID)
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(a.config.Issuer),
		jwt.WithAudience(a.config.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(a.config.ClockSkew),
		jwt.WithJSONNumber(),
		jwt.WithStrictDecoding(),
	)
	if err != nil || !token.Valid {
		return Identity{}, fmt.Errorf("%w: bearer token is invalid", ErrUnauthenticated)
	}
	userID, ok := stringClaim(claims, a.config.UserIDClaim)
	if !ok || !validIdentityValue(userID, 256) {
		return Identity{}, fmt.Errorf("%w: user identity claim is invalid", ErrUnauthenticated)
	}
	roles, err := rolesClaim(claims, a.config.RolesClaim)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: roles claim is invalid", ErrUnauthenticated)
	}
	return Identity{UserID: userID, Roles: roles, Source: "jwt"}, nil
}

func bearerToken(header string) (string, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) > maxTokenBytes {
		return "", fmt.Errorf("%w: bearer token is required", ErrUnauthenticated)
	}
	return parts[1], nil
}

func (a *JWTAuthenticator) keyFor(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	a.keysMu.RLock()
	key := a.keys[keyID]
	a.keysMu.RUnlock()
	if key != nil {
		return key, nil
	}

	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	a.keysMu.RLock()
	key = a.keys[keyID]
	a.keysMu.RUnlock()
	if key != nil {
		return key, nil
	}
	if !a.lastUnknownRefresh.IsZero() && time.Since(a.lastUnknownRefresh) < unknownKeyRetryWait {
		return nil, fmt.Errorf("JWT key ID is unknown")
	}
	a.lastUnknownRefresh = time.Now()
	if err := a.loadKeys(ctx); err != nil {
		return nil, err
	}
	a.keysMu.RLock()
	key = a.keys[keyID]
	a.keysMu.RUnlock()
	if key == nil {
		return nil, fmt.Errorf("JWT key ID is unknown")
	}
	return key, nil
}

func (a *JWTAuthenticator) loadKeys(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.config.JWKSURL, nil)
	if err != nil {
		return fmt.Errorf("create JWKS request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return fmt.Errorf("load JWT JWKS: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("load JWT JWKS: service returned %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxJWKSBytes+1))
	if err != nil {
		return fmt.Errorf("read JWT JWKS: %w", err)
	}
	if len(body) > maxJWKSBytes {
		return fmt.Errorf("JWT JWKS response is too large")
	}
	keys, err := parseJWKS(body)
	if err != nil {
		return err
	}
	a.keysMu.Lock()
	a.keys = keys
	a.keysMu.Unlock()
	return nil
}

func parseJWKS(data []byte) (map[string]*rsa.PublicKey, error) {
	var document struct {
		Keys []struct {
			KeyType string `json:"kty"`
			KeyID   string `json:"kid"`
			Use     string `json:"use"`
			Alg     string `json:"alg"`
			N       string `json:"n"`
			E       string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode JWT JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, item := range document.Keys {
		if item.KeyType != "RSA" || (item.Use != "" && item.Use != "sig") || (item.Alg != "" && item.Alg != "RS256") {
			continue
		}
		keyID := strings.TrimSpace(item.KeyID)
		if keyID == "" {
			continue
		}
		if _, exists := keys[keyID]; exists {
			return nil, fmt.Errorf("JWT JWKS contains duplicate key ID %q", keyID)
		}
		modulus, err := base64.RawURLEncoding.DecodeString(item.N)
		if err != nil || len(modulus) == 0 {
			return nil, fmt.Errorf("JWT JWKS contains an invalid RSA modulus")
		}
		exponentBytes, err := base64.RawURLEncoding.DecodeString(item.E)
		if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
			return nil, fmt.Errorf("JWT JWKS contains an invalid RSA exponent")
		}
		exponent := 0
		for _, value := range exponentBytes {
			exponent = exponent<<8 | int(value)
		}
		publicKey := &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}
		if publicKey.N.BitLen() < 2048 || publicKey.E < 3 || publicKey.E%2 == 0 {
			return nil, fmt.Errorf("JWT JWKS contains an unsafe RSA key")
		}
		keys[keyID] = publicKey
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("JWT JWKS contains no usable RS256 signing keys")
	}
	return keys, nil
}

func stringClaim(claims jwt.MapClaims, path string) (string, bool) {
	value, ok := claimValue(claims, path)
	if !ok {
		return "", false
	}
	result, ok := value.(string)
	return strings.TrimSpace(result), ok
}

func rolesClaim(claims jwt.MapClaims, path string) ([]string, error) {
	value, ok := claimValue(claims, path)
	if !ok {
		return []string{}, nil
	}
	values := make([]string, 0)
	switch typed := value.(type) {
	case string:
		values = append(values, typed)
	case []any:
		if len(typed) > 100 {
			return nil, fmt.Errorf("too many roles")
		}
		for _, item := range typed {
			role, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("role is not a string")
			}
			values = append(values, role)
		}
	default:
		return nil, fmt.Errorf("roles claim must be a string or array")
	}
	for _, value := range values {
		if !validIdentityValue(strings.TrimSpace(value), 128) {
			return nil, fmt.Errorf("role is invalid")
		}
	}
	return normalizeRoles(values), nil
}

func claimValue(claims jwt.MapClaims, path string) (any, bool) {
	var current any = map[string]any(claims)
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func validIdentityValue(value string, maxLength int) bool {
	if value == "" || len(value) > maxLength {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
