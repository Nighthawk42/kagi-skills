---
name: kagi-search
description: Web search via Kagi's Search API — high-quality, ad-free results with optional full page content, news/image/video/podcast modes, domain and date filtering, and Kagi lenses. Also surfaces small-web finds, infoboxes and related searches. Use when you need current information from the web or want to find authoritative sources on a topic.
---

# Kagi Search

Web search through [Kagi's Search API v1](https://help.kagi.com/kagi/api/search.html). Kagi is a paid, ad-free search engine, so results are ranked for relevance rather than engagement, which makes them markedly cleaner to feed to a model than ad-supported alternatives.

A static Go binary, so startup is instant and there is no runtime to install.

## Setup

1. Generate a **Kagi API v1 key** at <https://kagi.com/api/keys>
2. Export it:
   ```bash
   export KAGI_API_KEY="your-key-here"
   ```

> **A v1 key is required.** Keys issued for Kagi's older v0 API return `401 Unauthorized` here. If you see a 401, generate a fresh key at the link above.

**Cost:** $12 per 1,000 searches ($0.012 each). Adding `--extract` additionally bills at the Extract API rate of $4 per 1,000 pages. Check your balance at <https://kagi.com/api/billing>.

## Usage

```bash
kagi-search <query> [options]
```

The first run builds the binary from source if Go 1.26+ is present; otherwise it offers a checksum-verified download.

### Common cases

```bash
# Plain search
kagi-search "go generics tutorial"

# Fewer results, structured output
kagi-search "rust async runtime" -n 5 --json

# Restrict to specific sites
kagi-search "context cancellation" --site go.dev,pkg.go.dev

# Recent news
kagi-search "kubernetes release" --news --since week

# Search and read the top result in one request
kagi-search "attention is all you need" --extract 1
```

### Options

| Option | Description |
|---|---|
| `-n, --limit <1-1024>` | Max results **per category** (default: 10) |
| `--page <1-10>` | Result page |
| `-w, --workflow <name>` | `search` (default), `news`, `images`, `videos`, `podcasts` |
| `--news` `--images` `--videos` `--podcasts` | Shorthands for `--workflow` |
| `--only <names>` | Show only these categories, comma-separated |
| `--extract <1-10>` | Also fetch full page markdown for the top N results, inline |
| `--extract-chars <n>` | Truncate each extracted page (default: 20000, `0` = unlimited) |
| `--site <domains>` | Restrict to these domains (repeatable, comma-separated) |
| `--exclude-site <domains>` | Exclude these domains |
| `--keyword <words>` | Require these keywords |
| `--exclude-keyword <words>` | Exclude these keywords |
| `--file-type <ext>` | Restrict to a file type, e.g. `pdf` |
| `--since <day\|week\|month>` | Published within the last period |
| `--after <date>` / `--before <date>` | Published on or after / before, e.g. `2026-01-15` |
| `--region <code>` | ISO 3166-1 alpha-2 country code |
| `--unsafe` | Disable safe search |
| `--lens <id\|url>` | Apply a Kagi lens (see below) |
| `--json` | Emit JSON |
| `--markdown` | Emit Kagi's experimental markdown serialisation |
| `--meta` | Print timing and trace id to stderr |
| `--timeout <sec>` | HTTP timeout (default: 30) |
| `--api-timeout <0.5-4>` | Server-side search budget, in seconds |

`--site`, `--exclude-site`, `--keyword`, `--exclude-keyword`, `--file-type` and `--since` together form an inline lens, and are therefore **mutually exclusive with `--lens`** — Kagi silently ignores `lens_id` when inline lens options are present, so the tool refuses the combination rather than dropping your filters.

### Lenses

`--lens` narrows a search to a curated set of sources. Built-in IDs:

| ID | Lens |
|---|---|
| `1` | Forums |
| `2` | Academic |
| `15` | Programming |
| `29` | News 360 |
| `107` | Small Web |
| `120` | Recipes |

You can also pass a custom lens ID or full URL from <https://kagi.com/settings/lenses> (it must be set to shareable).

```bash
kagi-search "monad transformers" --lens 2        # academic sources
kagi-search "self-hosting" --lens 107            # small web
```

### Result categories

Kagi groups results by category, and a single query often returns several. Text output prints each as its own section; `--json` keys them under `collections`.

The ones you'll see most: `search` (web pages), `news`, `video`, `image`, `related_search` (query suggestions), `infobox` (summary of a person/place/thing), and **`interesting_finds`** — Kagi's small-web index of independent, non-commercial sites. Also possible: `direct_answer` (for maths and unit conversions), `adjacent_question`, `interesting_news`, `code`, `listicle`, `weather`, `public_records`, `package_tracking`, `web_archive`, `podcast`, `podcast_creator`.

Narrow the noise with `--only`:

```bash
kagi-search "steve jobs" --only search              # just web results
kagi-search "cool blogs" --only interesting_finds   # just small-web finds
kagi-search "rust" --only search,related_search
```

`--only` accepts the API's category names plus a few aliases: `web`/`results` → `search`, `related` → `related_search`, `smallweb`/`small-web`/`finds` → `interesting_finds`.

### Reading page content

`--extract N` replaces the snippet of each of the top N web results with the page's full markdown, in the same request. This is the cheapest way to search and read together.

```bash
kagi-search "python asyncio gather" --extract 2 --json
```

In `--json` output an extracted result carries `content` instead of `snippet`. Extracted markdown is deliberately **not** HTML-cleaned, so code samples survive intact.

To extract pages you already have URLs for, use the companion `kagi-extract` skill instead — and prefer it when you have several URLs, since it fetches up to 10 concurrently.

## JSON output

```json
{
  "query": "personal blog",
  "workflow": "search",
  "collections": {
    "search": [
      {
        "title": "Is This A Personal Blog?",
        "url": "https://axxuy.com/blog/2026/is-this-a-personal-blog/",
        "snippet": "...",
        "published": "2026-04-03",
        "language": "en"
      }
    ],
    "related_search": [{ "title": "personal blog website", "url": "/search?q=..." }]
  },
  "total": 12,
  "meta": { "trace": "637169aa...", "node": "us-central1", "ms": 1474 }
}
```

Titles and snippets are cleaned of HTML entities and highlight markup before output. `related_search` URLs are site-relative — use the `title`, which is the suggested query.

## Notes for agents

- `--limit` caps each category independently, so a `search` workflow can return more rows than `--limit` in total. Single-category workflows (`--news`, `--images`, …) cap the total.
- Prefer one request with `--extract` over a search followed by separate fetches.
- Pipe to `kagi-extract` when you need more pages than `--extract` covered:
  ```bash
  kagi-search "rust book" --json | jq -r '.collections.search[].url' | head -5 | kagi-extract
  ```
- `--meta` prints a trace id; quote it to <developers@kagi.com> when reporting an API problem.
- Transient `429` and `5xx` responses are retried automatically with backoff (`KAGI_MAX_RETRIES`, default 2).

## Environment

| Variable | Purpose |
|---|---|
| `KAGI_API_KEY` | **Required.** A Kagi API v1 key. |
| `KAGI_MAX_RETRIES` | Retries on 429/5xx (default: `2`). |
| `KAGI_SKILLS_ASSUME_YES` | Set to `1` to let the wrapper download a pre-built binary without prompting. |
