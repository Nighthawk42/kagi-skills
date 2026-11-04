---
name: kagi-extract
description: Fetch the full content of web pages as clean markdown using Kagi's Extract API. Handles up to 10 URLs per call, fetched concurrently. Use when you have a URL and need to read the actual page rather than a search snippet — articles, documentation, blog posts, papers.
---

# Kagi Extract

Turn web pages into clean markdown via [Kagi's Extract API](https://kagi.com/api/docs). Kagi does the fetching and boilerplate removal server-side, so you get readable content without running a scraper, a headless browser, or a readability library locally.

A static Go binary, so startup is instant and there is no runtime to install.

## Setup

1. Generate a **Kagi API v1 key** at <https://kagi.com/api/keys>
2. Export it:
   ```bash
   export KAGI_API_KEY="your-key-here"
   ```

> **A v1 key is required.** Keys issued for Kagi's older v0 API return `401 Unauthorized` here.

**Cost:** $4 per 1,000 pages ($0.004 each). Check your balance at <https://kagi.com/api/billing>.

## Usage

```bash
kagi-extract <url> [<url>...] [options]
```

```bash
# One page
kagi-extract https://go.dev/doc/go1.27

# Several at once — fetched concurrently, so far faster than looping
kagi-extract https://example.com/a https://example.com/b https://example.com/c

# Structured output
kagi-extract https://arxiv.org/abs/1706.03762 --json

# Cap the output size
kagi-extract https://en.wikipedia.org/wiki/Go_(programming_language) --max-chars 8000

# URLs from stdin
jq -r '.urls[]' links.json | kagi-extract
```

**Pass multiple URLs in one invocation whenever you can.** Kagi fetches them in parallel, so one call with five URLs is dramatically faster than five calls — and it is billed identically, per page.

### Options

| Option | Description |
|---|---|
| `--max-chars <n>` | Truncate each page (default: no limit) |
| `--json` | Emit JSON |
| `--markdown` | Emit Kagi's experimental markdown serialisation |
| `--no-headings` | Omit the `<!-- url -->` header between pages |
| `--meta` | Print timing and trace id to stderr |
| `--timeout <sec>` | HTTP timeout (default: 60) |
| `--api-timeout <0.5-10>` | Server-side budget for the whole batch, in seconds |

### Limits

- **10 URLs per invocation** — the API's limit. More than that is rejected before any request is made, so you are not billed for the mistake.
- URLs must be `http` or `https`. Anything else is rejected locally.
- `--api-timeout` is a budget for the entire batch, not per page, since Kagi fetches them concurrently.

## Output

With one URL, the markdown goes straight to stdout. With several, each is preceded by an HTML comment naming its URL:

```markdown
<!-- https://example.com/a -->

# Page A
...

<!-- https://example.com/b -->

# Page B
...
```

Use `--no-headings` to suppress those separators.

`--json`:

```json
{
  "pages": [
    { "url": "https://example.com/a", "markdown": "# Page A\n..." },
    { "url": "https://example.com/b", "error": "timed out" }
  ],
  "meta": { "trace": "7833cf00...", "node": "us-central1", "ms": 479 }
}
```

### Partial failures

A page can fail while others succeed, and the API reports that **in-band with a `200` status**. The tool therefore:

- writes every page that did extract to stdout,
- reports each failure on stderr,
- and exits non-zero **only if every page failed**.

So check stderr if you asked for several pages and got fewer back.

## Notes for agents

- If you are starting from a search rather than a URL, the `kagi-search` skill can extract inline with `--extract N`, which saves a round trip. Use this skill when you already have URLs.
- Chain from search results:
  ```bash
  kagi-search "rust async book" --json | jq -r '.collections.search[].url' | head -5 | kagi-extract
  ```
- Markdown is returned as-is, with no HTML post-processing, so code blocks and inline code survive intact.
- Extracted pages can be long — a single documentation page ran to ~34,000 characters in testing. Use `--max-chars` when feeding the output into a limited context.
- Transient `429` and `5xx` responses are retried automatically with backoff (`KAGI_MAX_RETRIES`, default 2).

## Environment

| Variable | Purpose |
|---|---|
| `KAGI_API_KEY` | **Required.** A Kagi API v1 key. |
| `KAGI_MAX_RETRIES` | Retries on 429/5xx (default: `2`). |
| `KAGI_SKILLS_ASSUME_YES` | Set to `1` to let the wrapper download a pre-built binary without prompting. |
