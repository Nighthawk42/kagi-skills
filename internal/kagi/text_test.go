package kagi

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCleanText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "Just words", "Just words"},
		{"apostrophe entity", "API Portal | Kagi&#39;s Docs", "API Portal | Kagi's Docs"},
		{"strong highlight", "The <strong>Kagi</strong> API Portal", "The Kagi API Portal"},
		{
			"infobox anchor",
			`Jobs co-founded <a href="https://en.wikipedia.org/wiki/Apple_Inc." data-wiki-article="/wiki/Apple_Inc." data-wiki-locale="en">Apple Inc.</a> with`,
			"Jobs co-founded Apple Inc. with",
		},
		{"amp entity", "Tom &amp; Jerry", "Tom & Jerry"},
		{"quot entity", "He said &quot;hi&quot;", `He said "hi"`},
		{"newlines collapse", "line one\n\n\tline   two\r\nthree", "line one line two three"},
		{"surrounding whitespace trimmed", "   padded  \n", "padded"},
		{"newline between inline tags collapses to a space", "<b>Bold</b>\n<em>Italic</em>", "Bold Italic"},
		{"escaped tag stays visible", "&lt;strong&gt;", "<strong>"},
		{"unknown tag kept", "Use <div> here", "Use <div> here"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanText(tc.in); got != tc.want {
				t.Errorf("CleanText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSuggestionTextCleansRelatedSearchTitle(t *testing.T) {
	r := Result{
		Title: "<strong>Steve</strong> Jobs &amp; Wozniak",
		URL:   "/search?q=steve+jobs",
	}
	if got := SuggestionText(r); got != "Steve Jobs & Wozniak" {
		t.Errorf("SuggestionText() = %q, want %q", got, "Steve Jobs & Wozniak")
	}
}

func TestTruncateRunesNoOpCases(t *testing.T) {
	const s = "hello world"
	if got := TruncateRunes(s, 0); got != s {
		t.Errorf("limit 0: got %q, want unchanged", got)
	}
	if got := TruncateRunes(s, -5); got != s {
		t.Errorf("limit -5: got %q, want unchanged", got)
	}
	if got := TruncateRunes(s, 100); got != s {
		t.Errorf("limit longer than input: got %q, want unchanged", got)
	}
	if got := TruncateRunes(s, len(s)); got != s {
		t.Errorf("limit equal to input: got %q, want unchanged", got)
	}
}

func TestTruncateRunesAppendsMarker(t *testing.T) {
	got := TruncateRunes("hello world", 5)
	want := "hello\n\n[... truncated]"
	if got != want {
		t.Errorf("TruncateRunes = %q, want %q", got, want)
	}
}

func TestTruncateRunesTrimsTrailingSpace(t *testing.T) {
	got := TruncateRunes("hello   world", 6)
	if !strings.HasPrefix(got, "hello\n\n[... truncated]") {
		t.Errorf("TruncateRunes = %q, want trailing spaces trimmed before marker", got)
	}
}

func TestTruncateRunesCutsOnRuneBoundaries(t *testing.T) {
	inputs := map[string]string{
		"CJK":   strings.Repeat("漢字テスト", 20),
		"emoji": strings.Repeat("🙂🚀é", 20),
		"mixed": "ab🙂cd漢efg",
	}
	for name, in := range inputs {
		t.Run(name, func(t *testing.T) {
			// "mixed" has 10 runes, so keep every limit below its length.
			for limit := 1; limit <= 9; limit++ {
				got := TruncateRunes(in, limit)
				if !utf8.ValidString(got) {
					t.Fatalf("limit %d produced invalid UTF-8: %q", limit, got)
				}
				body := strings.TrimSuffix(got, "\n\n[... truncated]")
				if utf8.RuneCountInString(body) != limit {
					t.Errorf("limit %d kept %d runes, want %d", limit, utf8.RuneCountInString(body), limit)
				}
				if !strings.HasPrefix(in, body) {
					t.Errorf("limit %d body %q is not a prefix of input", limit, body)
				}
			}
		})
	}
}

func TestDateOnly(t *testing.T) {
	cases := map[string]string{
		"2026-02-04T07:45:37Z": "2026-02-04",
		"2026-02-04":           "2026-02-04",
		"no-time-here":         "no-time-here",
		"":                     "",
		"T":                    "T",
	}
	for in, want := range cases {
		if got := DateOnly(in); got != want {
			t.Errorf("DateOnly(%q) = %q, want %q", in, got, want)
		}
	}
}
