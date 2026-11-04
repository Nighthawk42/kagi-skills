package kagi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const okSearchBody = `{"meta":{"trace":"t","ms":5,"node":"n"},"data":{"search":[{"url":"https://example.com","title":"Example","snippet":"A snippet"}]}}`

// newStub starts a server whose handler also counts every request it receives.
func newStub(t *testing.T, h http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

// newTestClient builds a Client directly, bypassing the environment, so tests
// can set MaxRetries precisely.
func newTestClient(base string, retries int) *Client {
	return &Client{
		APIKey:     "test-key",
		BaseURL:    base,
		HTTP:       &http.Client{Timeout: 5 * time.Second},
		MaxRetries: retries,
		UserAgent:  "kagi-skills-test",
	}
}

func writeJSON(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func asAPIError(t *testing.T, err error) *APIError {
	t.Helper()
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v (%T), want *APIError", err, err)
	}
	return apiErr
}

func TestSearchDecodesSuccessfulResponse(t *testing.T) {
	srv, count := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		var req SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		if req.Query != "rust" {
			t.Errorf("request query = %q, want %q", req.Query, "rust")
		}
		writeJSON(http.StatusOK, okSearchBody)(w, r)
	})

	c := newTestClient(srv.URL, 0)
	resp, err := c.Search(context.Background(), SearchRequest{Query: "rust"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if count.Load() != 1 {
		t.Errorf("request count = %d, want 1", count.Load())
	}
	if resp.Meta.Trace != "t" || resp.Meta.MS != 5 || resp.Meta.Node != "n" {
		t.Errorf("Meta = %+v, want trace t, ms 5, node n", resp.Meta)
	}
	if len(resp.Data.Search) != 1 {
		t.Fatalf("len(Data.Search) = %d, want 1", len(resp.Data.Search))
	}
	got := resp.Data.Search[0]
	if got.URL != "https://example.com" || got.Title != "Example" || got.Snippet != "A snippet" {
		t.Errorf("first result = %+v", got)
	}
}

func TestSearchSendsBearerAuthorization(t *testing.T) {
	srv, _ := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
		}
		if got := r.Header.Get("User-Agent"); got != "kagi-skills-test" {
			t.Errorf("User-Agent = %q, want %q", got, "kagi-skills-test")
		}
		writeJSON(http.StatusOK, okSearchBody)(w, r)
	})

	if _, err := newTestClient(srv.URL, 0).Search(context.Background(), SearchRequest{Query: "q"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
}

func TestNewClientUsesEnvironmentHostAndRetries(t *testing.T) {
	srv, count := newStub(t, writeJSON(http.StatusOK, okSearchBody))
	t.Setenv("KAGI_API_HOST", srv.URL+"/")
	t.Setenv("KAGI_MAX_RETRIES", "0")

	c := NewClient("k", time.Second)
	if c.BaseURL != srv.URL {
		t.Errorf("BaseURL = %q, want %q (trailing slash trimmed)", c.BaseURL, srv.URL)
	}
	if c.MaxRetries != 0 {
		t.Errorf("MaxRetries = %d, want 0", c.MaxRetries)
	}
	if _, err := c.Search(context.Background(), SearchRequest{Query: "q"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if count.Load() != 1 {
		t.Errorf("request count = %d, want 1", count.Load())
	}
}

func TestNewClientDefaults(t *testing.T) {
	t.Setenv("KAGI_API_HOST", "")
	t.Setenv("KAGI_MAX_RETRIES", "")
	c := NewClient("k", time.Second)
	if c.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL, DefaultBaseURL)
	}
	if c.MaxRetries != 2 {
		t.Errorf("MaxRetries = %d, want 2", c.MaxRetries)
	}
}

func TestMaxRetriesFromEnv(t *testing.T) {
	cases := map[string]int{
		"":    2,
		"5":   5,
		"0":   0,
		"-1":  2,
		"abc": 2,
	}
	for raw, want := range cases {
		t.Run("value="+raw, func(t *testing.T) {
			t.Setenv("KAGI_MAX_RETRIES", raw)
			if got := maxRetriesFromEnv(); got != want {
				t.Errorf("maxRetriesFromEnv() with %q = %d, want %d", raw, got, want)
			}
		})
	}
}

func TestUnauthorizedError(t *testing.T) {
	srv, _ := newStub(t, writeJSON(http.StatusUnauthorized,
		`{"meta":{"trace":"abc"},"error":[{"code":"auth.invalid","message":"Invalid token"}]}`))

	_, err := newTestClient(srv.URL, 0).Search(context.Background(), SearchRequest{Query: "q"})
	apiErr := asAPIError(t, err)
	if !apiErr.Unauthorized() {
		t.Error("Unauthorized() = false, want true")
	}
	msg := apiErr.Error()
	for _, want := range []string{"https://kagi.com/api/keys", "not accepted"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, missing %q", msg, want)
		}
	}
}

func TestNonUnauthorizedIsNotUnauthorized(t *testing.T) {
	srv, _ := newStub(t, writeJSON(http.StatusForbidden, `{"error":[{"code":"x","message":"no"}]}`))
	_, err := newTestClient(srv.URL, 0).Search(context.Background(), SearchRequest{Query: "q"})
	if asAPIError(t, err).Unauthorized() {
		t.Error("Unauthorized() = true for a 403")
	}
}

func TestSingularErrorEnvelopeSurfacesMessageAndTrace(t *testing.T) {
	srv, _ := newStub(t, writeJSON(http.StatusBadRequest,
		`{"meta":{"trace":"x"},"error":[{"code":"search.limit_invalid","message":"Limit must be between 1 and 1024"}]}`))

	_, err := newTestClient(srv.URL, 0).Search(context.Background(), SearchRequest{Query: "q"})
	msg := asAPIError(t, err).Error()
	for _, want := range []string{"Limit must be between 1 and 1024", "x"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, missing %q", msg, want)
		}
	}
	if !strings.Contains(msg, "trace id: x") {
		t.Errorf("Error() = %q, missing trace id", msg)
	}
}

func TestPluralErrorEnvelopeDecodesOnExtract(t *testing.T) {
	srv, _ := newStub(t, writeJSON(http.StatusNotFound,
		`{"meta":{"trace":"nf"},"errors":[{"code":"route.missing","message":"No such endpoint"}]}`))

	_, err := newTestClient(srv.URL, 0).Extract(context.Background(), ExtractRequest{
		Pages: []PageInput{{URL: "https://example.com"}},
	})
	apiErr := asAPIError(t, err)
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Error(), "No such endpoint") {
		t.Errorf("Error() = %q, missing message from plural envelope", apiErr.Error())
	}
}

func TestErrorDetailLocationIsRendered(t *testing.T) {
	srv, _ := newStub(t, writeJSON(http.StatusBadRequest,
		`{"errors":[{"code":"pages.invalid","message":"bad url","location":"pages[0].url"}]}`))

	_, err := newTestClient(srv.URL, 0).Extract(context.Background(), ExtractRequest{
		Pages: []PageInput{{URL: "https://example.com"}},
	})
	if msg := asAPIError(t, err).Error(); !strings.Contains(msg, "bad url (at pages[0].url)") {
		t.Errorf("Error() = %q, want location rendered", msg)
	}
}

func TestErrorDetailFallsBackToCodeWithoutMessage(t *testing.T) {
	srv, _ := newStub(t, writeJSON(http.StatusBadRequest,
		`{"error":[{"code":"search.query_empty"}]}`))

	_, err := newTestClient(srv.URL, 0).Search(context.Background(), SearchRequest{Query: "q"})
	if msg := asAPIError(t, err).Error(); !strings.Contains(msg, "search.query_empty") {
		t.Errorf("Error() = %q, want code used as fallback", msg)
	}
}

func TestNonRetryableStatusIsSingleAttempt(t *testing.T) {
	srv, count := newStub(t, writeJSON(http.StatusBadRequest, `{"error":[{"code":"x","message":"bad"}]}`))

	_, err := newTestClient(srv.URL, 3).Search(context.Background(), SearchRequest{Query: "q"})
	asAPIError(t, err)
	if count.Load() != 1 {
		t.Errorf("request count = %d, want 1", count.Load())
	}
}

func TestRetryableStatusesAreRetriedThenFail(t *testing.T) {
	t.Parallel()

	statuses := []int{
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}
	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			srv, count := newStub(t, writeJSON(status, `{"error":[{"code":"busy","message":"try later"}]}`))

			_, err := newTestClient(srv.URL, 1).Search(context.Background(), SearchRequest{Query: "q"})
			apiErr := asAPIError(t, err)
			if apiErr.StatusCode != status {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, status)
			}
			if count.Load() != 2 {
				t.Errorf("request count = %d, want 2 (1 attempt + MaxRetries 1)", count.Load())
			}
		})
	}
}

func TestTransientFailureThenSuccess(t *testing.T) {
	var n atomic.Int32
	srv, count := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			writeJSON(http.StatusServiceUnavailable, `{"error":[{"code":"busy","message":"x"}]}`)(w, r)
			return
		}
		writeJSON(http.StatusOK, okSearchBody)(w, r)
	})

	resp, err := newTestClient(srv.URL, 2).Search(context.Background(), SearchRequest{Query: "q"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if count.Load() != 2 {
		t.Errorf("request count = %d, want 2", count.Load())
	}
	if len(resp.Data.Search) != 1 {
		t.Errorf("len(Data.Search) = %d, want 1", len(resp.Data.Search))
	}
}

func TestRetryAfterHeaderIsHonoured(t *testing.T) {
	var n atomic.Int32
	srv, count := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			writeJSON(http.StatusTooManyRequests, `{"error":[{"code":"rate","message":"slow down"}]}`)(w, r)
			return
		}
		writeJSON(http.StatusOK, okSearchBody)(w, r)
	})

	start := time.Now()
	if _, err := newTestClient(srv.URL, 1).Search(context.Background(), SearchRequest{Query: "q"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	elapsed := time.Since(start)
	if count.Load() != 2 {
		t.Errorf("request count = %d, want 2", count.Load())
	}
	// The default backoff for the first retry is 500ms, so waiting at least
	// close to a second proves the Retry-After hint was used.
	if elapsed < 900*time.Millisecond {
		t.Errorf("elapsed = %v, want >= ~1s (Retry-After: 1 not honoured)", elapsed)
	}
}

func TestCancelledContextDoesNotRetry(t *testing.T) {
	srv, count := newStub(t, writeJSON(http.StatusServiceUnavailable, `{"error":[{"code":"busy","message":"x"}]}`))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := newTestClient(srv.URL, 5).Search(ctx, SearchRequest{Query: "q"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Search returned nil error after context cancellation")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("elapsed = %v, want prompt return", elapsed)
	}
	if count.Load() != 1 {
		t.Errorf("request count = %d, want 1 (no retries after cancel)", count.Load())
	}
}

func TestPreCancelledContextMakesNoRequest(t *testing.T) {
	srv, count := newStub(t, writeJSON(http.StatusOK, okSearchBody))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := newTestClient(srv.URL, 2).Search(ctx, SearchRequest{Query: "q"}); err == nil {
		t.Fatal("Search returned nil error for a cancelled context")
	}
	if count.Load() != 0 {
		t.Errorf("request count = %d, want 0", count.Load())
	}
}

func TestUnparseableErrorBodyIsTruncated(t *testing.T) {
	body := strings.Repeat("Z", 1000)
	srv, _ := newStub(t, writeJSON(http.StatusBadRequest, body))

	_, err := newTestClient(srv.URL, 0).Search(context.Background(), SearchRequest{Query: "q"})
	msg := asAPIError(t, err).Error()
	if got := strings.Count(msg, "Z"); got != maxErrorBodyChars {
		t.Errorf("echoed %d body characters, want %d", got, maxErrorBodyChars)
	}
	if !strings.Contains(msg, "...") {
		t.Errorf("Error() = %q, want truncation marker", msg)
	}
}

func TestUnparseableSuccessBodyIsAnError(t *testing.T) {
	srv, _ := newStub(t, writeJSON(http.StatusOK, `<html>not json</html>`))
	_, err := newTestClient(srv.URL, 0).Search(context.Background(), SearchRequest{Query: "q"})
	if err == nil || !strings.Contains(err.Error(), "parsing Kagi response") {
		t.Errorf("error = %v, want parsing error", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := map[string]time.Duration{
		"":        0,
		"0":       0,
		"-3":      0,
		"7":       7 * time.Second,
		"garbage": 0,
	}
	for raw, want := range cases {
		t.Run("value="+raw, func(t *testing.T) {
			h := http.Header{}
			if raw != "" {
				h.Set("Retry-After", raw)
			}
			if got := parseRetryAfter(h); got != want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", raw, got, want)
			}
		})
	}
}

func TestBackoffIsCapped(t *testing.T) {
	if got := backoff(1, 0); got != 500*time.Millisecond {
		t.Errorf("backoff(1, 0) = %v, want 500ms", got)
	}
	if got := backoff(2, 0); got != time.Second {
		t.Errorf("backoff(2, 0) = %v, want 1s", got)
	}
	if got := backoff(30, 0); got != 10*time.Second {
		t.Errorf("backoff(30, 0) = %v, want cap of 10s", got)
	}
	if got := backoff(1, time.Hour); got != 10*time.Second {
		t.Errorf("backoff with huge hint = %v, want cap of 10s", got)
	}
}
