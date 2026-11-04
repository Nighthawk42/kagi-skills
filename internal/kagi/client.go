// Package kagi is a client for the Kagi API v1 (https://kagi.com/api/docs).
//
// It covers the two endpoints the v1 API exposes: POST /search and
// POST /extract. The older v0 endpoints (fastgpt, summarize, enrich) are not
// part of v1 and are not implemented here.
package kagi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the production Kagi API v1 endpoint.
	DefaultBaseURL = "https://kagi.com/api/v1"

	// maxResponseBytes caps how much of a response body we will read. Inline
	// extraction can return a few hundred KB per page, so this is generous.
	maxResponseBytes = 64 << 20

	// maxErrorBodyChars caps how much of an unparseable error body we echo.
	maxErrorBodyChars = 500
)

// retryableStatuses are the HTTP statuses worth retrying. 429 is documented by
// Kagi as "rate limited or usage limit exhausted"; the 5xx entries cover
// transient upstream failures.
var retryableStatuses = map[int]bool{
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

// Client calls the Kagi API v1.
type Client struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client

	// MaxRetries is the number of additional attempts made after a retryable
	// failure. Zero means a single attempt.
	MaxRetries int

	// UserAgent identifies this client to Kagi. Unlike the v0 tooling this
	// replaces, we do not impersonate a browser: every request here goes to
	// Kagi's own API, which has no reason to be told we are Chrome.
	UserAgent string
}

// Meta is the debugging envelope Kagi returns with every response.
//
// Note for anyone porting from the v0 API: v0 included an api_balance field
// here, which this project used to cache and report via a "balance" command.
// v1 does not return a balance by any route, so that command no longer exists.
// Check your balance at https://kagi.com/api/billing instead.
type Meta struct {
	// Trace identifies the request. Quote it when contacting Kagi support.
	Trace string `json:"trace,omitempty"`
	// Node is the hostname that served the request.
	Node string `json:"node,omitempty"`
	// MS is how long Kagi took to fulfil the request, excluding round trip.
	MS int `json:"ms,omitempty"`
}

// NewClient builds a Client with sensible defaults.
//
// The base URL may be overridden with KAGI_API_HOST, which exists for testing
// against a local stub and is not something end users need.
func NewClient(apiKey string, timeout time.Duration) *Client {
	base := strings.TrimSpace(os.Getenv("KAGI_API_HOST"))
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{
		APIKey:     apiKey,
		BaseURL:    strings.TrimSuffix(base, "/"),
		HTTP:       &http.Client{Timeout: timeout},
		MaxRetries: maxRetriesFromEnv(),
		UserAgent:  "kagi-skills",
	}
}

func maxRetriesFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("KAGI_MAX_RETRIES"))
	if raw == "" {
		return 2
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 2
	}
	return n
}

// APIError is a structured error returned by the Kagi API.
type APIError struct {
	// StatusCode is the HTTP status.
	StatusCode int
	// Details holds the namespaced error entries Kagi returned, if any.
	Details []ErrorDetail
	// Meta carries the trace ID, when the response included one.
	Meta Meta
	// Body is the raw response body, used when it could not be parsed.
	Body string

	// retryAfter carries a server-supplied Retry-After hint, so that backoff
	// can honour it. Callers have no use for it.
	retryAfter time.Duration
}

// ErrorDetail is one entry from a v1 error envelope.
type ErrorDetail struct {
	// Code is a namespaced error code such as "search.limit_invalid".
	Code string `json:"code"`
	// URL points at error documentation. Kagi notes these may 404 for now.
	URL string `json:"url,omitempty"`
	// Message is human-readable.
	Message string `json:"message,omitempty"`
	// Location names the offending request field, e.g. "pages[0].url".
	Location string `json:"location,omitempty"`
}

// errorEnvelope decodes a v1 error response.
//
// Kagi is inconsistent about the key: POST /search returns "error" while
// /extract and endpoint-not-found responses return "errors". We accept both.
type errorEnvelope struct {
	Meta   Meta          `json:"meta"`
	Error  []ErrorDetail `json:"error"`
	Errors []ErrorDetail `json:"errors"`
}

func (e *errorEnvelope) details() []ErrorDetail {
	if len(e.Error) > 0 {
		return e.Error
	}
	return e.Errors
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Kagi API returned HTTP %d", e.StatusCode)

	switch {
	case len(e.Details) > 0:
		parts := make([]string, 0, len(e.Details))
		for _, d := range e.Details {
			msg := d.Message
			if msg == "" {
				msg = d.Code
			}
			if d.Location != "" {
				msg = fmt.Sprintf("%s (at %s)", msg, d.Location)
			}
			parts = append(parts, msg)
		}
		fmt.Fprintf(&b, ": %s", strings.Join(parts, "; "))
	case e.Body != "":
		fmt.Fprintf(&b, ": %s", truncate(e.Body, maxErrorBodyChars))
	}

	// A 401 is overwhelmingly likely to mean a v0 token, since this tool used
	// to speak v0 and Kagi issues the two key types separately. Say so, rather
	// than leaving the user staring at "Unauthorized".
	if e.StatusCode == http.StatusUnauthorized {
		b.WriteString("\n\nThis endpoint needs a Kagi API v1 key. Keys issued for the" +
			"\nolder v0 API are not accepted. Generate a v1 key at" +
			"\nhttps://kagi.com/api/keys and set KAGI_API_KEY to it.")
	}
	if e.Meta.Trace != "" {
		fmt.Fprintf(&b, "\n\n(trace id: %s — quote this to developers@kagi.com)", e.Meta.Trace)
	}
	return b.String()
}

// Unauthorized reports whether the error was an authentication failure.
func (e *APIError) Unauthorized() bool { return e.StatusCode == http.StatusUnauthorized }

// ErrNoAPIKey is returned when KAGI_API_KEY is unset or empty.
var ErrNoAPIKey = errors.New(
	"KAGI_API_KEY is not set.\n\n" +
		"Generate a Kagi API v1 key at https://kagi.com/api/keys, then:\n" +
		"  export KAGI_API_KEY=\"your-key-here\"\n\n" +
		"Note that keys for the older v0 API will not work.")

// APIKeyFromEnv reads and validates KAGI_API_KEY.
func APIKeyFromEnv() (string, error) {
	key := strings.TrimSpace(os.Getenv("KAGI_API_KEY"))
	if key == "" {
		return "", ErrNoAPIKey
	}
	return key, nil
}

// postJSON sends reqBody to path and decodes the response into out.
//
// When raw is non-nil it receives the undecoded body instead, which is how the
// experimental format:"markdown" responses are handled — those are not JSON.
func (c *Client) postJSON(ctx context.Context, path string, reqBody, out any, raw *string) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	endpoint, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return fmt.Errorf("building request URL: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, backoff(attempt, lastRetryAfter(lastErr))); err != nil {
				return err
			}
		}

		status, body, header, err := c.do(ctx, endpoint, payload)
		if err != nil {
			// Transport errors (DNS, connection reset, timeout) are worth one
			// more try, but a cancelled context never is.
			if ctx.Err() != nil {
				return err
			}
			lastErr = err
			continue
		}

		if status >= 200 && status < 300 {
			if raw != nil {
				*raw = string(body)
				return nil
			}
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("parsing Kagi response: %w", err)
			}
			return nil
		}

		apiErr := newAPIError(status, body)
		if !retryableStatuses[status] {
			return apiErr
		}
		apiErr.retryAfter = parseRetryAfter(header)
		lastErr = apiErr
	}

	if lastErr == nil {
		lastErr = errors.New("request failed with no error recorded")
	}
	return lastErr
}

// do performs a single request attempt, returning the status, body and headers.
func (c *Client) do(ctx context.Context, endpoint string, payload []byte) (int, []byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/markdown")
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return resp.StatusCode, nil, resp.Header, fmt.Errorf("reading Kagi response: %w", err)
	}
	return resp.StatusCode, body, resp.Header, nil
}

func newAPIError(status int, body []byte) *APIError {
	out := &APIError{StatusCode: status}
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err == nil {
		out.Details = env.details()
		out.Meta = env.Meta
	}
	if len(out.Details) == 0 {
		out.Body = strings.TrimSpace(string(body))
	}
	return out
}

// parseRetryAfter reads a Retry-After header, which RFC 9110 allows to be
// either a delay in seconds or an HTTP date.
func parseRetryAfter(header http.Header) time.Duration {
	raw := strings.TrimSpace(header.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if when, err := http.ParseTime(raw); err == nil {
		if d := time.Until(when); d > 0 {
			return d
		}
	}
	return 0
}

func lastRetryAfter(err error) time.Duration {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.retryAfter
	}
	return 0
}

// backoff returns the delay before the given attempt (1-based), honouring a
// server-supplied hint when there is one.
func backoff(attempt int, hint time.Duration) time.Duration {
	if hint > 0 {
		if hint > 10*time.Second {
			return 10 * time.Second
		}
		return hint
	}
	d := time.Duration(math.Pow(2, float64(attempt-1))*500) * time.Millisecond
	if d > 10*time.Second {
		return 10 * time.Second
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "..."
}
