package kagi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Extract API limits documented by the API.
const (
	// MaxExtractPages is how many URLs one /extract request accepts.
	MaxExtractPages = 10

	// MinExtractTimeout and MaxExtractTimeout bound the server-side budget for
	// the whole bulk fetch, which Kagi performs concurrently.
	MinExtractTimeout = 0.5
	MaxExtractTimeout = 10.0
)

// ExtractRequest is the body of POST /extract.
type ExtractRequest struct {
	Pages []PageInput `json:"pages"`
	// Timeout is the server-side budget in seconds for the entire bulk fetch.
	// Out-of-range values are clamped by Kagi rather than rejected.
	Timeout float64 `json:"timeout,omitempty"`
	Format  string  `json:"format,omitempty"`
}

// PageInput is one page to extract.
type PageInput struct {
	URL string `json:"url"`
}

// PageOutput is the extracted content for one page. Exactly one of Markdown or
// Error is meaningful.
type PageOutput struct {
	URL      string `json:"url"`
	Markdown string `json:"markdown,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ExtractResponse is the body of a successful POST /extract.
//
// Note that per-page failures are reported in-band via PageOutput.Error with a
// 200 status: a successful response does not mean every page was extracted.
type ExtractResponse struct {
	Meta   Meta          `json:"meta"`
	Data   []PageOutput  `json:"data"`
	Errors []ErrorDetail `json:"errors,omitempty"`
}

// Extract fetches the given URLs and returns their content as markdown.
func (c *Client) Extract(ctx context.Context, req ExtractRequest) (*ExtractResponse, error) {
	if len(req.Pages) == 0 {
		return nil, errors.New("at least one URL is required")
	}
	if len(req.Pages) > MaxExtractPages {
		return nil, fmt.Errorf("at most %d URLs per request, got %d", MaxExtractPages, len(req.Pages))
	}
	var out ExtractResponse
	if err := c.postJSON(ctx, "extract", req, &out, nil); err != nil {
		return nil, err
	}
	return &out, nil
}

// ExtractMarkdown fetches the given URLs and returns Kagi's experimental
// markdown serialisation verbatim, rather than JSON.
func (c *Client) ExtractMarkdown(ctx context.Context, req ExtractRequest) (string, error) {
	if len(req.Pages) == 0 {
		return "", errors.New("at least one URL is required")
	}
	req.Format = "markdown"
	var raw string
	if err := c.postJSON(ctx, "extract", req, nil, &raw); err != nil {
		return "", err
	}
	return raw, nil
}

// ValidateExtractURL checks a URL before spending a request on it.
//
// The API documents pages as requiring HTTPS. We accept http:// too and let
// Kagi make the final call, but reject anything that is clearly not a web URL,
// since a malformed entry would otherwise be billed as a failed page.
func ValidateExtractURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("empty URL")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", trimmed, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("invalid URL %q: scheme must be http or https, got %q", trimmed, u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid URL %q: missing host", trimmed)
	}
	return u.String(), nil
}
