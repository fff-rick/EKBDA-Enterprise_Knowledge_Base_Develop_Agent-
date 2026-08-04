package release

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ConnectorConfig struct {
	Enabled bool
	BaseURL string
	Token   string
	Timeout time.Duration
}

type HTTPConnector struct {
	enabled bool
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPConnector(config ConnectorConfig) (*HTTPConnector, error) {
	connector := &HTTPConnector{enabled: config.Enabled}
	if !config.Enabled {
		return connector, nil
	}
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || strings.TrimSpace(config.Token) == "" || config.Timeout < time.Second || config.Timeout > 5*time.Minute {
		return nil, ErrInvalidInput
	}
	connector.baseURL, connector.token, connector.client = parsed.String(), config.Token, &http.Client{Timeout: config.Timeout}
	return connector, nil
}

func (c *HTTPConnector) Enabled() bool { return c != nil && c.enabled }
func (c *HTTPConnector) Trigger(ctx context.Context, request TriggerRequest) (ProviderRun, error) {
	return c.post(ctx, "/api/v1/runs", request.ReleaseID, request)
}
func (c *HTTPConnector) Rollback(ctx context.Context, request RollbackRequest) (ProviderRun, error) {
	return c.post(ctx, "/api/v1/rollbacks", request.ReleaseID+":rollback", request)
}

func (c *HTTPConnector) post(ctx context.Context, path, idempotencyKey string, payload any) (ProviderRun, error) {
	if !c.Enabled() {
		return ProviderRun{}, ErrDisabled
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ProviderRun{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return ProviderRun{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response, err := c.client.Do(request)
	if err != nil {
		return ProviderRun{}, err
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, 64<<10)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, limited)
		return ProviderRun{}, fmt.Errorf("provider status %d", response.StatusCode)
	}
	var run ProviderRun
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&run); err != nil {
		return ProviderRun{}, errors.New("invalid provider response")
	}
	return run, nil
}

type WebhookVerifier struct {
	secret []byte
	maxAge time.Duration
	now    func() time.Time
}

func NewWebhookVerifier(secret string, maxAge time.Duration) (*WebhookVerifier, error) {
	if len(secret) < 32 || maxAge < time.Minute || maxAge > 15*time.Minute {
		return nil, ErrInvalidInput
	}
	return &WebhookVerifier{secret: []byte(secret), maxAge: maxAge, now: time.Now}, nil
}

func (v *WebhookVerifier) Verify(timestamp, signature string, body []byte) error {
	if v == nil || len(v.secret) == 0 || len(body) == 0 {
		return ErrInvalidInput
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return ErrInvalidInput
	}
	eventTime := time.Unix(seconds, 0)
	age := v.now().Sub(eventTime)
	if age < -v.maxAge || age > v.maxAge {
		return ErrInvalidInput
	}
	provided := strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	providedBytes, err := hex.DecodeString(provided)
	if err != nil {
		return ErrInvalidInput
	}
	mac := hmac.New(sha256.New, v.secret)
	_, _ = mac.Write([]byte(strings.TrimSpace(timestamp) + "."))
	_, _ = mac.Write(body)
	if !hmac.Equal(providedBytes, mac.Sum(nil)) {
		return ErrInvalidInput
	}
	return nil
}

func SignWebhook(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
