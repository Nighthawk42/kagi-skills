package kagi

import (
	"html"
	"regexp"
	"strings"
)

// markupTags matches the inline markup Kagi embeds in titles and snippets.
//
// Live responses contain highlight tags (<strong>Kagi</strong>), and infobox
// snippets carry full anchors with data attributes:
//
//	<a href="https://en.wikipedia.org/wiki/Apple_Inc." data-wiki-article="...">Apple Inc.</a>
//
// We strip the tags and keep their text content.
var markupTags = regexp.MustCompile(`(?is)</?(?:strong|b|em|i|mark|span|a|br|wbr)(?:\s[^>]*)?/?>`)

// collapseSpace squeezes runs of whitespace, including the newlines left
// behind by stripped block markup.
var collapseSpace = regexp.MustCompile(`\s+`)

// CleanText makes a Kagi title or snippet fit for display: markup stripped,
// HTML entities resolved, whitespace collapsed.
//
// Do not apply this to inline-extracted page markdown. That content is already
// markdown rather than HTML, and stripping angle-bracket spans would corrupt
// code samples such as Box<dyn Future<Output = ()>> or Vec<String>.
func CleanText(s string) string {
	if s == "" {
		return ""
	}
	out := markupTags.ReplaceAllString(s, "")
	out = html.UnescapeString(out)
	out = collapseSpace.ReplaceAllString(out, " ")
	return strings.TrimSpace(out)
}

// TruncateRunes cuts s to at most limit runes, appending an ellipsis marker
// when it had to cut. A limit of zero or less means no truncation.
//
// Counting runes rather than bytes keeps multi-byte characters intact.
func TruncateRunes(s string, limit int) string {
	if limit <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return strings.TrimRight(string(r[:limit]), " \t\n") + "\n\n[... truncated]"
}

// DateOnly reduces an RFC 3339 timestamp to its date part. Kagi returns times
// like 2026-02-04T07:45:37Z, and the clock portion is rarely useful.
func DateOnly(ts string) string {
	if i := strings.IndexByte(ts, 'T'); i > 0 {
		return ts[:i]
	}
	return ts
}

// SuggestionText returns the display text for a related_search result.
//
// These results carry the suggestion in Title and a site-relative URL
// (/search?q=steve+jobs) in URL, so the title is what callers want.
func SuggestionText(r Result) string { return CleanText(r.Title) }
