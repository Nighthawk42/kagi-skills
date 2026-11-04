package kagi

import (
	"context"
	"errors"
)

// Workflow selects which category of results Kagi returns.
type Workflow = string

// Valid Workflow values.
const (
	WorkflowSearch   Workflow = "search"
	WorkflowImages   Workflow = "images"
	WorkflowVideos   Workflow = "videos"
	WorkflowNews     Workflow = "news"
	WorkflowPodcasts Workflow = "podcasts"
)

// Workflows lists every valid workflow, in the order the API documents them.
var Workflows = []Workflow{
	WorkflowSearch, WorkflowImages, WorkflowVideos, WorkflowNews, WorkflowPodcasts,
}

// Request limits documented by the API.
const (
	MinLimit = 1
	MaxLimit = 1024

	MinPage = 1
	MaxPage = 10

	// MinAPITimeout and MaxAPITimeout bound the server-side time budget.
	MinAPITimeout = 0.5
	MaxAPITimeout = 4.0

	// MaxExtractCount is how many search results may be extracted inline.
	MaxExtractCount = 10
)

// SearchRequest is the body of POST /search.
//
// Every field but Query is optional; omitted fields let Kagi apply its own
// current defaults, which is preferable to pinning values that may drift.
type SearchRequest struct {
	Query    string `json:"query"`
	Workflow string `json:"workflow,omitempty"`
	Format   string `json:"format,omitempty"`
	LensID   string `json:"lens_id,omitempty"`
	Lens     *Lens  `json:"lens,omitempty"`

	// Timeout is the server-side budget in seconds (0.5-4). It is unrelated to
	// the HTTP client timeout.
	Timeout float64 `json:"timeout,omitempty"`
	Page    int     `json:"page,omitempty"`
	Limit   int     `json:"limit,omitempty"`

	Filters *Filters       `json:"filters,omitempty"`
	Extract *ExtractInline `json:"extract,omitempty"`
}

// Lens describes an inline lens. Options set here take precedence over
// operators in the query itself, such as site:.
type Lens struct {
	SitesIncluded    []string `json:"sites_included,omitempty"`
	SitesExcluded    []string `json:"sites_excluded,omitempty"`
	KeywordsIncluded []string `json:"keywords_included,omitempty"`
	KeywordsExcluded []string `json:"keywords_excluded,omitempty"`
	FileType         string   `json:"file_type,omitempty"`
	TimeAfter        string   `json:"time_after,omitempty"`
	TimeBefore       string   `json:"time_before,omitempty"`
	TimeRelative     string   `json:"time_relative,omitempty"`
	SearchRegion     string   `json:"search_region,omitempty"`
}

// IsZero reports whether the lens carries no options, in which case it should
// be omitted from the request entirely.
func (l *Lens) IsZero() bool {
	return l == nil || (len(l.SitesIncluded) == 0 && len(l.SitesExcluded) == 0 &&
		len(l.KeywordsIncluded) == 0 && len(l.KeywordsExcluded) == 0 &&
		l.FileType == "" && l.TimeAfter == "" && l.TimeBefore == "" &&
		l.TimeRelative == "" && l.SearchRegion == "")
}

// Filters narrows results. Anything set here overrides the equivalent lens
// option, per the API docs.
type Filters struct {
	Region     string `json:"region,omitempty"`
	After      string `json:"after,omitempty"`
	Before     string `json:"before,omitempty"`
	SafeSearch *bool  `json:"safe_search,omitempty"`
}

// IsZero reports whether the filter set is empty.
func (f *Filters) IsZero() bool {
	return f == nil || (f.Region == "" && f.After == "" && f.Before == "" && f.SafeSearch == nil)
}

// ExtractInline asks Kagi to fetch page content for the top Count results and
// return it, as markdown, in place of each result's snippet.
//
// This is billed at the Extract API rate on top of the search.
type ExtractInline struct {
	Count   int     `json:"count,omitempty"`
	Timeout float64 `json:"timeout,omitempty"`
}

// Image accompanies a result: a video thumbnail or a page's lead image.
type Image struct {
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// Result is a single search result.
//
// Title and Snippet arrive containing HTML entities and highlight markup
// (&#39;, <strong>...</strong>, and in infoboxes full <a href> tags). Use
// CleanText before displaying them. Inline-extracted markdown must NOT be
// cleaned: stripping tags would mangle code such as Box<dyn Future<Output = ()>>.
type Result struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Snippet string `json:"snippet,omitempty"`
	// Time is when the resource was created or last updated (RFC 3339).
	// Note this was called "published" in the v0 API.
	Time  string `json:"time,omitempty"`
	Image *Image `json:"image,omitempty"`
	// Props holds arbitrary per-result metadata: language, group_id, and for
	// adjacent_question results the question itself.
	Props map[string]any `json:"props,omitempty"`
}

// Question returns the props.question value carried by adjacent_question
// results, if present.
func (r Result) Question() string {
	if q, ok := r.Props["question"].(string); ok {
		return q
	}
	return ""
}

// SearchData groups results by category. Kagi documents more collections than
// a single query usually returns, so most of these are empty most of the time.
type SearchData struct {
	Search           []Result `json:"search,omitempty"`
	Image            []Result `json:"image,omitempty"`
	Video            []Result `json:"video,omitempty"`
	News             []Result `json:"news,omitempty"`
	Podcast          []Result `json:"podcast,omitempty"`
	PodcastCreator   []Result `json:"podcast_creator,omitempty"`
	AdjacentQuestion []Result `json:"adjacent_question,omitempty"`
	DirectAnswer     []Result `json:"direct_answer,omitempty"`
	InterestingNews  []Result `json:"interesting_news,omitempty"`
	InterestingFinds []Result `json:"interesting_finds,omitempty"`
	Infobox          []Result `json:"infobox,omitempty"`
	Code             []Result `json:"code,omitempty"`
	PackageTracking  []Result `json:"package_tracking,omitempty"`
	PublicRecords    []Result `json:"public_records,omitempty"`
	Weather          []Result `json:"weather,omitempty"`
	RelatedSearch    []Result `json:"related_search,omitempty"`
	Listicle         []Result `json:"listicle,omitempty"`
	WebArchive       []Result `json:"web_archive,omitempty"`
}

// SearchResponse is the body of a successful POST /search.
type SearchResponse struct {
	Meta Meta       `json:"meta"`
	Data SearchData `json:"data"`
}

// Collection pairs a category name with its results.
type Collection struct {
	// Name is the API's field name, e.g. "related_search".
	Name string
	// Label is a human-readable heading, e.g. "Related Searches".
	Label   string
	Results []Result
}

// Collections returns the non-empty collections in a stable, useful order:
// primary results first, then the extras that the official Kagi MCP server
// discards (infoboxes, small-web finds, related searches).
func (d *SearchData) Collections() []Collection {
	all := []Collection{
		{"direct_answer", "Direct Answer", d.DirectAnswer},
		{"infobox", "Infobox", d.Infobox},
		{"weather", "Weather", d.Weather},
		{"search", "Results", d.Search},
		{"news", "News", d.News},
		{"image", "Images", d.Image},
		{"video", "Videos", d.Video},
		{"podcast", "Podcasts", d.Podcast},
		{"podcast_creator", "Podcast Creators", d.PodcastCreator},
		{"code", "Code", d.Code},
		{"listicle", "Listicles", d.Listicle},
		{"public_records", "Public Records", d.PublicRecords},
		{"package_tracking", "Package Tracking", d.PackageTracking},
		{"web_archive", "Web Archive", d.WebArchive},
		{"interesting_news", "Interesting News", d.InterestingNews},
		{"interesting_finds", "Interesting Finds (Small Web)", d.InterestingFinds},
		{"adjacent_question", "Adjacent Questions", d.AdjacentQuestion},
		{"related_search", "Related Searches", d.RelatedSearch},
	}
	out := make([]Collection, 0, len(all))
	for _, c := range all {
		if len(c.Results) > 0 {
			out = append(out, c)
		}
	}
	return out
}

// Total counts every result across all collections.
func (d *SearchData) Total() int {
	n := 0
	for _, c := range d.Collections() {
		n += len(c.Results)
	}
	return n
}

// Search performs a web search.
func (c *Client) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	if req.Query == "" {
		return nil, errors.New("query is required")
	}
	var out SearchResponse
	if err := c.postJSON(ctx, "search", req, &out, nil); err != nil {
		return nil, err
	}
	return &out, nil
}

// SearchMarkdown performs a search and returns Kagi's experimental markdown
// serialisation verbatim, rather than JSON.
func (c *Client) SearchMarkdown(ctx context.Context, req SearchRequest) (string, error) {
	if req.Query == "" {
		return "", errors.New("query is required")
	}
	req.Format = "markdown"
	var raw string
	if err := c.postJSON(ctx, "search", req, nil, &raw); err != nil {
		return "", err
	}
	return raw, nil
}
