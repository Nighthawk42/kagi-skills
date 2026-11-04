package kagi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestValidateExtractURLAccepts(t *testing.T) {
	cases := map[string]string{
		"https://example.com/a":   "https://example.com/a",
		"http://example.com":      "http://example.com",
		"  https://example.com  ": "https://example.com",
	}
	for in, want := range cases {
		got, err := ValidateExtractURL(in)
		if err != nil {
			t.Errorf("ValidateExtractURL(%q) error = %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ValidateExtractURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateExtractURLRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
		// wantSubstr is text the error must contain. For cases with a URL
		// to name, it is that URL.
		wantSubstr string
	}{
		{"empty", "", ""},
		{"whitespace only", "   \t ", ""},
		{"ftp scheme", "ftp://example.com/file", "ftp://example.com/file"},
		{"file scheme", "file:///etc/passwd", "file:///etc/passwd"},
		{"javascript scheme", "javascript:alert(1)", "javascript:alert(1)"},
		{"no host", "https://", "https://"},
		{"no scheme", "example.com/page", "example.com/page"},
		{"malformed", "http://[::1", "http://[::1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateExtractURL(tc.in)
			if err == nil {
				t.Fatalf("ValidateExtractURL(%q) returned nil error", tc.in)
			}
			if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error = %q, want it to name %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestExtractRejectsWrongPageCountWithoutRequest(t *testing.T) {
	srv, count := newStub(t, writeJSON(http.StatusOK, `{"data":[]}`))
	c := newTestClient(srv.URL, 0)

	if _, err := c.Extract(context.Background(), ExtractRequest{}); err == nil {
		t.Error("Extract with zero pages returned nil error")
	}

	tooMany := make([]PageInput, MaxExtractPages+1)
	for i := range tooMany {
		tooMany[i] = PageInput{URL: "https://example.com/" + string(rune('a'+i))}
	}
	if _, err := c.Extract(context.Background(), ExtractRequest{Pages: tooMany}); err == nil {
		t.Errorf("Extract with %d pages returned nil error", len(tooMany))
	} else if !strings.Contains(err.Error(), "11") {
		t.Errorf("error = %q, want it to report the count 11", err.Error())
	}

	if _, err := c.ExtractMarkdown(context.Background(), ExtractRequest{}); err == nil {
		t.Error("ExtractMarkdown with zero pages returned nil error")
	}

	if count.Load() != 0 {
		t.Errorf("request count = %d, want 0", count.Load())
	}
}

func TestExtractAcceptsExactlyMaxPages(t *testing.T) {
	srv, count := newStub(t, writeJSON(http.StatusOK, `{"data":[]}`))
	pages := make([]PageInput, MaxExtractPages)
	for i := range pages {
		pages[i] = PageInput{URL: "https://example.com"}
	}
	if _, err := newTestClient(srv.URL, 0).Extract(context.Background(), ExtractRequest{Pages: pages}); err != nil {
		t.Fatalf("Extract with %d pages: %v", len(pages), err)
	}
	if count.Load() != 1 {
		t.Errorf("request count = %d, want 1", count.Load())
	}
}

func TestExtractDecodesPerPageErrorsInBand(t *testing.T) {
	body := `{
		"meta": {"trace": "tr"},
		"data": [
			{"url": "https://ok.example.com", "markdown": "# Hello\n\nWorld"},
			{"url": "https://bad.example.com", "error": "Page could not be fetched"}
		]
	}`
	srv, _ := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		var req ExtractRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		if len(req.Pages) != 2 {
			t.Errorf("sent %d pages, want 2", len(req.Pages))
		}
		writeJSON(http.StatusOK, body)(w, r)
	})

	resp, err := newTestClient(srv.URL, 0).Extract(context.Background(), ExtractRequest{
		Pages: []PageInput{{URL: "https://ok.example.com"}, {URL: "https://bad.example.com"}},
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if resp.Meta.Trace != "tr" {
		t.Errorf("Meta.Trace = %q, want tr", resp.Meta.Trace)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("len(Data) = %d, want 2", len(resp.Data))
	}
	ok, bad := resp.Data[0], resp.Data[1]
	if ok.Markdown != "# Hello\n\nWorld" || ok.Error != "" {
		t.Errorf("ok page = %+v", ok)
	}
	if bad.Error != "Page could not be fetched" || bad.Markdown != "" {
		t.Errorf("bad page = %+v", bad)
	}
}

func TestExtractDecodesTopLevelErrors(t *testing.T) {
	body := `{
		"meta": {},
		"data": [],
		"errors": [{"code": "extract.timeout", "message": "Budget exhausted", "location": "timeout"}]
	}`
	srv, _ := newStub(t, writeJSON(http.StatusOK, body))

	resp, err := newTestClient(srv.URL, 0).Extract(context.Background(), ExtractRequest{
		Pages: []PageInput{{URL: "https://example.com"}},
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(resp.Errors) != 1 {
		t.Fatalf("len(Errors) = %d, want 1", len(resp.Errors))
	}
	e := resp.Errors[0]
	if e.Code != "extract.timeout" || e.Message != "Budget exhausted" || e.Location != "timeout" {
		t.Errorf("Errors[0] = %+v", e)
	}
}

func TestExtractRequestSendsPagesAndOmitsEmptyOptions(t *testing.T) {
	b, err := json.Marshal(ExtractRequest{Pages: []PageInput{{URL: "https://example.com"}}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := m["pages"]; !ok {
		t.Errorf("pages key missing: %s", b)
	}
	for _, key := range []string{"timeout", "format"} {
		if _, ok := m[key]; ok {
			t.Errorf("key %q emitted for zero value: %s", key, b)
		}
	}
}
