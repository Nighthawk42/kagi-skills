// Command kagi-extract fetches web pages as markdown via the Kagi Extract API.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Nighthawk42/kagi-skills/internal/cli"
	"github.com/Nighthawk42/kagi-skills/internal/kagi"
)

const defaultHTTPTimeout = 60

type options struct {
	urls        []string
	maxChars    int
	apiTimeout  float64
	httpTimeout int
	jsonOut     bool
	markdownOut bool
	showMeta    bool
	noHeadings  bool
}

type jsonOutput struct {
	Pages []jsonPage `json:"pages"`
	Meta  kagi.Meta  `json:"meta"`
}

type jsonPage struct {
	URL      string `json:"url"`
	Markdown string `json:"markdown,omitempty"`
	Error    string `json:"error,omitempty"`
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
	if len(argv) > 0 {
		switch argv[0] {
		case "--version", "-v":
			fmt.Printf("kagi-extract %s\n", cli.Version)
			return nil
		case "-h", "--help":
			usage()
			return nil
		}
	}

	opts, err := parse(argv)
	if err != nil {
		return err
	}
	if len(opts.urls) == 0 {
		usage()
		return errors.New("at least one URL is required")
	}
	if len(opts.urls) > kagi.MaxExtractPages {
		return fmt.Errorf("at most %d URLs per request, got %d (the Kagi Extract API's limit)",
			kagi.MaxExtractPages, len(opts.urls))
	}

	pages := make([]kagi.PageInput, 0, len(opts.urls))
	for _, raw := range opts.urls {
		clean, err := kagi.ValidateExtractURL(raw)
		if err != nil {
			return err
		}
		pages = append(pages, kagi.PageInput{URL: clean})
	}

	apiKey, err := kagi.APIKeyFromEnv()
	if err != nil {
		return err
	}

	ctx, stop := cli.SignalContext()
	defer stop()

	client := kagi.NewClient(apiKey, time.Duration(opts.httpTimeout)*time.Second)
	req := kagi.ExtractRequest{Pages: pages, Timeout: opts.apiTimeout}

	if opts.markdownOut {
		md, err := client.ExtractMarkdown(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println(strings.TrimSpace(md))
		return nil
	}

	resp, err := client.Extract(ctx, req)
	if err != nil {
		return err
	}

	if opts.jsonOut {
		return writeJSON(opts, resp)
	}
	return printText(opts, resp)
}

func parse(argv []string) (*options, error) {
	opts := &options{httpTimeout: defaultHTTPTimeout}

	a := cli.NewArgs(argv)
	for a.Next() {
		var err error
		switch {
		case a.Is("-h", "--help"):
			usage()
			return nil, cli.ErrUsage
		case a.Is("--max-chars"):
			opts.maxChars, err = a.Int()
		case a.Is("--api-timeout"):
			opts.apiTimeout, err = a.FloatInRange(kagi.MinExtractTimeout, kagi.MaxExtractTimeout)
		case a.Is("--timeout"):
			opts.httpTimeout, err = a.IntInRange(1, 600)
		case a.Is("--json"):
			opts.jsonOut = true
		case a.Is("--markdown"):
			opts.markdownOut = true
		case a.Is("--meta"):
			opts.showMeta = true
		case a.Is("--no-headings"):
			opts.noHeadings = true
		default:
			return nil, a.Unknown()
		}
		if err != nil {
			return nil, err
		}
	}

	if opts.jsonOut && opts.markdownOut {
		return nil, errors.New("--json and --markdown are mutually exclusive")
	}

	opts.urls = a.Positionals

	// With no URLs on the command line, read them from stdin when it is piped,
	// so that `... | kagi-extract` works.
	if len(opts.urls) == 0 {
		piped, err := readURLsFromStdin()
		if err != nil {
			return nil, err
		}
		opts.urls = piped
	}
	return opts, nil
}

// stdinIsPiped reports whether stdin has data piped into it rather than being
// an interactive terminal. A Stat failure tells us nothing useful either way,
// so it counts as "not piped" and the caller reports the missing URL instead.
func stdinIsPiped() bool {
	stat, err := os.Stdin.Stat()
	return err == nil && stat.Mode()&os.ModeCharDevice == 0
}

func readURLsFromStdin() ([]string, error) {
	if !stdinIsPiped() {
		return nil, nil
	}
	var urls []string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		for field := range strings.FieldsSeq(scanner.Text()) {
			urls = append(urls, field)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading URLs from stdin: %w", err)
	}
	return urls, nil
}

// printText writes each page's markdown. Per-page failures are reported to
// stderr and set a non-zero exit, but never discard the pages that did work.
func printText(opts *options, resp *kagi.ExtractResponse) error {
	var failed int

	for i, page := range resp.Data {
		if page.Error != "" {
			fmt.Fprintf(os.Stderr, "Failed to extract %s: %s\n", page.URL, page.Error)
			failed++
			continue
		}
		if !opts.noHeadings && len(resp.Data) > 1 {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("<!-- %s -->\n\n", page.URL)
		}
		fmt.Println(kagi.TruncateRunes(strings.TrimSpace(page.Markdown), opts.maxChars))
	}

	for _, e := range resp.Errors {
		msg := e.Message
		if msg == "" {
			msg = e.Code
		}
		fmt.Fprintln(os.Stderr, "Extract error:", msg)
		failed++
	}

	if opts.showMeta {
		fmt.Fprintf(os.Stderr, "[%d pages in %dms | node %s | trace %s]\n",
			len(resp.Data), resp.Meta.MS, resp.Meta.Node, resp.Meta.Trace)
	}

	if failed > 0 && failed >= len(resp.Data) {
		return errors.New("extraction failed for every requested page")
	}
	return nil
}

func writeJSON(opts *options, resp *kagi.ExtractResponse) error {
	out := jsonOutput{
		Pages: make([]jsonPage, 0, len(resp.Data)),
		Meta:  resp.Meta,
	}
	for _, page := range resp.Data {
		out.Pages = append(out.Pages, jsonPage{
			URL:      page.URL,
			Markdown: kagi.TruncateRunes(page.Markdown, opts.maxChars),
			Error:    page.Error,
		})
	}
	for _, e := range resp.Errors {
		msg := e.Message
		if msg == "" {
			msg = e.Code
		}
		out.Pages = append(out.Pages, jsonPage{Error: msg})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

func usage() {
	fmt.Print(`kagi-extract — fetch web pages as markdown via the Kagi Extract API

Usage:
  kagi-extract <url> [<url>...] [options]
  kagi-extract --json < urls.txt
  kagi-extract --version

Up to 10 URLs per invocation (the API's limit). Kagi fetches them
concurrently, so passing several at once is much faster than looping.

Options:
      --max-chars <n>        Truncate each page (default: no limit)
      --json                 Emit JSON
      --markdown             Emit Kagi's experimental markdown serialisation
      --no-headings          Omit the per-URL comment header when extracting
                             multiple pages
      --meta                 Print timing and trace id to stderr
      --timeout <sec>        HTTP timeout (default: 60)
      --api-timeout <0.5-10> Server-side budget for the whole batch, in seconds

Environment:
  KAGI_API_KEY               Required. A Kagi API v1 key from
                             https://kagi.com/api/keys
  KAGI_MAX_RETRIES           Retries on 429/5xx (default: 2)

Exit status is non-zero only if every page failed; partial failures are
reported on stderr while successful pages are still written to stdout.

Examples:
  kagi-extract https://go.dev/doc/go1.27
  kagi-extract https://example.com/a https://example.com/b --json
  kagi-search "rust book" --json | jq -r '.collections.search[].url' | head -5 | kagi-extract
`)
}
