// Command kagi-search queries the Kagi Search API v1.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Nighthawk42/kagi-skills/internal/cli"
	"github.com/Nighthawk42/kagi-skills/internal/kagi"
)

const (
	defaultLimit        = 10
	defaultHTTPTimeout  = 30
	defaultExtractChars = 20000
)

type options struct {
	query string

	workflow     string
	limit        int
	page         int
	extractCount int
	extractChars int

	includeDomains []string
	excludeDomains []string
	keywordsInc    []string
	keywordsExc    []string
	fileType       string
	timeRelative   string
	after          string
	before         string
	region         string
	lensID         string
	unsafeSearch   bool

	apiTimeout  float64
	httpTimeout int

	jsonOut     bool
	markdownOut bool
	only        []string
	showMeta    bool
}

// jsonOutput is the --json shape. It is deliberately flatter than the API's
// own response: collections become a map keyed by category name, with titles
// and snippets already cleaned of markup.
type jsonOutput struct {
	Query       string                  `json:"query"`
	Workflow    string                  `json:"workflow"`
	Collections map[string][]jsonResult `json:"collections"`
	Total       int                     `json:"total"`
	Meta        kagi.Meta               `json:"meta"`
}

type jsonResult struct {
	Title     string `json:"title,omitempty"`
	URL       string `json:"url,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
	Content   string `json:"content,omitempty"`
	Published string `json:"published,omitempty"`
	ImageURL  string `json:"image_url,omitempty"`
	Question  string `json:"question,omitempty"`
	Language  string `json:"language,omitempty"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, cli.ErrUsage) {
			return
		}
		// A closed stdout pipe is normal in a shell pipeline; say nothing.
		if cli.IsBrokenPipe(err) {
			return
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) == 0 {
		usage()
		return errors.New("a query is required")
	}

	// Accept an optional leading "search" subcommand for continuity with the
	// previous versions of this tool.
	switch argv[0] {
	case "--version", "-v":
		fmt.Printf("kagi-search %s\n", cli.Version)
		return nil
	case "-h", "--help":
		usage()
		return nil
	case "search":
		argv = argv[1:]
	case "balance":
		return errors.New("the balance command has been removed: the Kagi API v1 does not\n" +
			"report a credit balance. Check it at https://kagi.com/api/billing")
	case "content":
		return errors.New("the content command has been replaced by the kagi-extract tool,\n" +
			"which uses Kagi's Extract API instead of scraping pages locally")
	}

	opts, err := parse(argv)
	if err != nil {
		return err
	}
	if opts.query == "" {
		usage()
		return errors.New("a query is required")
	}

	apiKey, err := kagi.APIKeyFromEnv()
	if err != nil {
		return err
	}

	ctx, stop := cli.SignalContext()
	defer stop()

	client := kagi.NewClient(apiKey, time.Duration(opts.httpTimeout)*time.Second)
	req := opts.toRequest()

	if opts.markdownOut {
		md, err := client.SearchMarkdown(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println(strings.TrimSpace(md))
		return nil
	}

	resp, err := client.Search(ctx, req)
	if err != nil {
		return err
	}

	collections := filterCollections(resp.Data.Collections(), opts.only)

	if opts.jsonOut {
		return writeJSON(opts, resp, collections)
	}
	printText(opts, resp, collections)
	return nil
}

func parse(argv []string) (*options, error) {
	opts := &options{
		limit:        defaultLimit,
		httpTimeout:  defaultHTTPTimeout,
		extractChars: defaultExtractChars,
	}

	a := cli.NewArgs(argv)
	for a.Next() {
		var err error
		switch {
		case a.Is("-h", "--help"):
			usage()
			return nil, cli.ErrUsage

		case a.Is("-n", "--limit"):
			opts.limit, err = a.IntInRange(kagi.MinLimit, kagi.MaxLimit)
		case a.Is("--page"):
			opts.page, err = a.IntInRange(kagi.MinPage, kagi.MaxPage)
		case a.Is("-w", "--workflow"):
			opts.workflow, err = a.Enum(kagi.Workflows...)
		case a.Is("--news"):
			opts.workflow = kagi.WorkflowNews
		case a.Is("--images"):
			opts.workflow = kagi.WorkflowImages
		case a.Is("--videos"):
			opts.workflow = kagi.WorkflowVideos
		case a.Is("--podcasts"):
			opts.workflow = kagi.WorkflowPodcasts

		case a.Is("--extract"):
			opts.extractCount, err = a.IntInRange(1, kagi.MaxExtractCount)
		case a.Is("--extract-chars"):
			opts.extractChars, err = a.Int()

		case a.Is("--site", "--include-domain"):
			opts.includeDomains, err = a.StringList(opts.includeDomains)
		case a.Is("--exclude-site", "--exclude-domain"):
			opts.excludeDomains, err = a.StringList(opts.excludeDomains)
		case a.Is("--keyword"):
			opts.keywordsInc, err = a.StringList(opts.keywordsInc)
		case a.Is("--exclude-keyword"):
			opts.keywordsExc, err = a.StringList(opts.keywordsExc)
		case a.Is("--file-type"):
			opts.fileType, err = a.String()
		case a.Is("--since"):
			opts.timeRelative, err = a.Enum("day", "week", "month")
		case a.Is("--after"):
			opts.after, err = a.String()
		case a.Is("--before"):
			opts.before, err = a.String()
		case a.Is("--region"):
			opts.region, err = a.String()
		case a.Is("--lens"):
			opts.lensID, err = a.String()
		case a.Is("--unsafe"):
			opts.unsafeSearch = true

		case a.Is("--api-timeout"):
			opts.apiTimeout, err = a.FloatInRange(kagi.MinAPITimeout, kagi.MaxAPITimeout)
		case a.Is("--timeout"):
			opts.httpTimeout, err = a.IntInRange(1, 600)

		case a.Is("--json"):
			opts.jsonOut = true
		case a.Is("--markdown"):
			opts.markdownOut = true
		case a.Is("--only"):
			opts.only, err = a.StringList(opts.only)
		case a.Is("--meta"):
			opts.showMeta = true

		default:
			return nil, a.Unknown()
		}
		if err != nil {
			return nil, err
		}
	}

	opts.query = a.Query()

	if opts.jsonOut && opts.markdownOut {
		return nil, errors.New("--json and --markdown are mutually exclusive")
	}
	if opts.timeRelative != "" && (opts.after != "" || opts.before != "") {
		return nil, errors.New("--since is mutually exclusive with --after and --before")
	}
	// Kagi ignores lens_id when inline lens options are also present, which
	// would silently drop the user's filters. Refuse instead.
	if opts.lensID != "" && opts.hasInlineLens() {
		return nil, errors.New("--lens is mutually exclusive with --site, --exclude-site,\n" +
			"--keyword, --exclude-keyword, --file-type and --since (Kagi ignores the\n" +
			"lens when these are set). Use one or the other")
	}
	return opts, nil
}

func (o *options) hasInlineLens() bool {
	return len(o.includeDomains) > 0 || len(o.excludeDomains) > 0 ||
		len(o.keywordsInc) > 0 || len(o.keywordsExc) > 0 ||
		o.fileType != "" || o.timeRelative != ""
}

func (o *options) toRequest() kagi.SearchRequest {
	req := kagi.SearchRequest{
		Query:    o.query,
		Workflow: o.workflow,
		Limit:    o.limit,
		Page:     o.page,
		Timeout:  o.apiTimeout,
		LensID:   o.lensID,
	}

	lens := &kagi.Lens{
		SitesIncluded:    o.includeDomains,
		SitesExcluded:    o.excludeDomains,
		KeywordsIncluded: o.keywordsInc,
		KeywordsExcluded: o.keywordsExc,
		FileType:         o.fileType,
		TimeRelative:     o.timeRelative,
	}
	if !lens.IsZero() {
		req.Lens = lens
	}

	filters := &kagi.Filters{Region: o.region, After: o.after, Before: o.before}
	if o.unsafeSearch {
		off := false
		filters.SafeSearch = &off
	}
	if !filters.IsZero() {
		req.Filters = filters
	}

	if o.extractCount > 0 {
		req.Extract = &kagi.ExtractInline{Count: o.extractCount}
	}
	return req
}

// filterCollections applies --only. Names are matched against both the API
// field name and a few friendly aliases.
func filterCollections(all []kagi.Collection, only []string) []kagi.Collection {
	if len(only) == 0 {
		return all
	}
	wanted := make(map[string]bool, len(only))
	for _, name := range only {
		n := strings.ToLower(strings.TrimSpace(name))
		wanted[n] = true
		switch n {
		case "results", "web":
			wanted["search"] = true
		case "related":
			wanted["related_search"] = true
		case "smallweb", "small-web", "finds":
			wanted["interesting_finds"] = true
		}
	}
	out := make([]kagi.Collection, 0, len(all))
	for _, c := range all {
		if wanted[c.Name] {
			out = append(out, c)
		}
	}
	return out
}

// isExtracted reports whether a result's snippet holds inline-extracted page
// markdown rather than a search snippet. Kagi only applies extraction to the
// top N results of the primary "search" collection.
func isExtracted(opts *options, collection string, rank int) bool {
	return opts.extractCount > 0 && collection == "search" && rank < opts.extractCount
}

func printText(opts *options, resp *kagi.SearchResponse, collections []kagi.Collection) {
	if len(collections) == 0 {
		fmt.Fprintln(os.Stderr, "No results found.")
		return
	}

	for _, c := range collections {
		fmt.Printf("=== %s ===\n\n", c.Label)

		// Related searches and adjacent questions are suggestions, not pages;
		// a bare list reads far better than a titled block per item.
		if c.Name == "related_search" {
			for _, r := range c.Results {
				fmt.Printf("  - %s\n", kagi.SuggestionText(r))
			}
			fmt.Println()
			continue
		}

		for i, r := range c.Results {
			if q := r.Question(); q != "" {
				fmt.Printf("Q: %s\n", kagi.CleanText(q))
			}
			fmt.Printf("%d. %s\n", i+1, kagi.CleanText(r.Title))
			fmt.Printf("   %s\n", r.URL)
			if r.Time != "" {
				fmt.Printf("   Published: %s\n", kagi.DateOnly(r.Time))
			}
			if r.Image != nil && r.Image.URL != "" && c.Name == "image" {
				fmt.Printf("   Image: %s\n", r.Image.URL)
			}
			if r.Snippet != "" {
				if isExtracted(opts, c.Name, i) {
					fmt.Printf("\n%s\n", kagi.TruncateRunes(r.Snippet, opts.extractChars))
				} else {
					fmt.Printf("   %s\n", kagi.CleanText(r.Snippet))
				}
			}
			fmt.Println()
		}
	}

	if opts.showMeta {
		fmt.Fprintf(os.Stderr, "[%d results in %dms | node %s | trace %s]\n",
			resp.Data.Total(), resp.Meta.MS, resp.Meta.Node, resp.Meta.Trace)
	}
}

func writeJSON(opts *options, resp *kagi.SearchResponse, collections []kagi.Collection) error {
	out := jsonOutput{
		Query:       opts.query,
		Workflow:    orDefault(opts.workflow, kagi.WorkflowSearch),
		Collections: make(map[string][]jsonResult, len(collections)),
		Meta:        resp.Meta,
	}

	for _, c := range collections {
		items := make([]jsonResult, 0, len(c.Results))
		for i, r := range c.Results {
			item := jsonResult{
				Title:     kagi.CleanText(r.Title),
				URL:       r.URL,
				Published: kagi.DateOnly(r.Time),
				Question:  kagi.CleanText(r.Question()),
			}
			if r.Image != nil {
				item.ImageURL = r.Image.URL
			}
			if lang, ok := r.Props["language"].(string); ok {
				item.Language = lang
			}
			if r.Snippet != "" {
				if isExtracted(opts, c.Name, i) {
					item.Content = kagi.TruncateRunes(r.Snippet, opts.extractChars)
				} else {
					item.Snippet = kagi.CleanText(r.Snippet)
				}
			}
			items = append(items, item)
		}
		out.Collections[c.Name] = items
		out.Total += len(items)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func usage() {
	fmt.Print(`kagi-search — web search via the Kagi Search API v1

Usage:
  kagi-search <query> [options]
  kagi-search --version

Result options:
  -n, --limit <1-1024>      Max results per category (default: 10)
      --page <1-10>         Result page
  -w, --workflow <name>     search (default), news, images, videos, podcasts
      --news                Shorthand for --workflow news
      --images              Shorthand for --workflow images
      --videos              Shorthand for --workflow videos
      --podcasts            Shorthand for --workflow podcasts
      --only <names>        Show only these categories (comma-separated).
                            e.g. search, news, related_search, infobox,
                            interesting_finds. Aliases: web, related, smallweb

Page content:
      --extract <1-10>      Also fetch full page markdown for the top N web
                            results, inline. Billed at the Extract API rate.
      --extract-chars <n>   Truncate each extracted page (default: 20000, 0 = no limit)

Filtering (these form a lens; mutually exclusive with --lens):
      --site <domains>      Restrict to these domains (repeatable, comma-separated)
      --exclude-site <d>    Exclude these domains
      --keyword <words>     Require these keywords
      --exclude-keyword <w> Exclude these keywords
      --file-type <ext>     Restrict to a file type, e.g. pdf
      --since <period>      day, week or month

Filtering (applied on top of any lens):
      --after <date>        Published on or after, e.g. 2026-01-15
      --before <date>       Published on or before
      --region <code>       ISO 3166-1 alpha-2 country code
      --unsafe              Disable safe search
      --lens <id|url>       Apply a saved or built-in Kagi lens. Built-ins:
                            1 Forums, 2 Academic, 15 Programming,
                            29 News 360, 107 Small Web, 120 Recipes

Output:
      --json                Emit JSON
      --markdown            Emit Kagi's experimental markdown serialisation
      --meta                Print timing and trace id to stderr
      --timeout <sec>       HTTP timeout (default: 30)
      --api-timeout <0.5-4> Server-side search budget in seconds

Environment:
  KAGI_API_KEY              Required. A Kagi API v1 key from
                            https://kagi.com/api/keys
  KAGI_MAX_RETRIES          Retries on 429/5xx (default: 2)

Examples:
  kagi-search "go generics tutorial"
  kagi-search "rust async" --site doc.rust-lang.org --limit 5
  kagi-search "kubernetes 1.32 release" --news --since week
  kagi-search "attention is all you need" --extract 1 --json
  kagi-search "small web blogs" --only interesting_finds,related_search
`)
}
