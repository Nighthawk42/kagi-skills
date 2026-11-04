# kagi-skills

[![CI](https://github.com/Nighthawk42/kagi-skills/actions/workflows/ci.yml/badge.svg)](https://github.com/Nighthawk42/kagi-skills/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/Nighthawk42/kagi-skills)](https://github.com/Nighthawk42/kagi-skills/releases/latest)
[![Go Version](https://img.shields.io/badge/go-1.26+-blue)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Command-line tools that give any AI agent access to [Kagi's](https://kagi.com) Search and Extract APIs: web search and clean page-to-markdown extraction.

Static Go binaries with **zero dependencies**, so they work with any agent that can run a shell command — [Claude Code](https://github.com/anthropics/claude-code), [pi](https://github.com/mariozechner/pi), [Gemini CLI](https://github.com/google-gemini/gemini-cli), [Codex](https://github.com/openai/codex), [Cursor](https://cursor.com) — and equally well from a plain shell script or a cron job.

## Should you use this, or Kagi's official MCP server?

Be aware that Kagi now ships an [official MCP server](https://github.com/kagisearch/kagimcp), hosted at `https://mcp.kagi.com/mcp`. **If you only use MCP-capable agents, that is probably the better choice** — it is maintained by Kagi against their own OpenAPI spec, and for Claude Code the whole install is one command:

```bash
claude mcp add kagi https://mcp.kagi.com/mcp --transport http \
  --header "Authorization: Bearer $KAGI_API_KEY" --scope user
```

This project earns its place in four specific situations:

1. **Agents and tools that don't speak MCP.** These are just executables; anything that can run a command can use them.
2. **Shell pipelines and scripts.** `kagi-search "..." --json | jq ...` composes in a way an MCP tool cannot.
3. **No Python runtime.** A single static binary with instant startup, rather than a Python process and its dependencies.
4. **Result categories the official server discards.** It returns only `search`, `news`, `video`, `podcast` and `image` — its source comments note that "related searches, infoboxes, creator listings and the like are dropped". These tools expose all of them, including **`interesting_finds`**, Kagi's small-web index of independent, non-commercial sites.

That's a portability and ergonomics argument, not a capability one. If none of those apply to you, use the official server.

## Tools

| Tool | What it does | Kagi API | Price |
| --- | --- | --- | --- |
| **kagi-search** | Web search, with optional inline page content | [Search](https://help.kagi.com/kagi/api/search.html) | $12 / 1,000 queries |
| **kagi-extract** | Up to 10 URLs → clean markdown, fetched concurrently | [Extract](https://kagi.com/api/docs) | $4 / 1,000 pages |

Each tool's full usage lives in its `SKILL.md`: [kagi-search](kagi-search/SKILL.md), [kagi-extract](kagi-extract/SKILL.md).

## Get an API key

1. Create a Kagi account at <https://kagi.com/signup>
2. Generate a key at <https://kagi.com/api/keys>
3. Add credit at <https://kagi.com/api/billing>
4. Export it:

```bash
# bash/zsh
export KAGI_API_KEY="your-api-key-here"

# fish
set -Ux KAGI_API_KEY "your-api-key-here"
```

One key works for both tools.

> ### You need a **v1** key
>
> Kagi runs two generations of API. These tools use **v1** (`/api/v1/...`, `Authorization: Bearer`). Keys issued for the older **v0** API return `401 Unauthorized` and cannot be used here — generate a fresh key at <https://kagi.com/api/keys>.
>
> This also means the v0-only Kagi APIs — FastGPT, Universal Summarizer and Enrichment — are out of reach of a v1 key. Earlier versions of this project wrapped all three; see [Removed tools](#removed-tools) below.

## Quick start

```bash
git clone https://github.com/Nighthawk42/kagi-skills
cd kagi-skills
make build
```

Then symlink the skills into your agent's skills directory:

### Claude Code

```bash
# bash/zsh
for skill in kagi-search kagi-extract; do ln -s "$(pwd)/$skill" ~/.claude/skills/"$skill"; done

# fish
for skill in kagi-search kagi-extract; ln -s (pwd)/$skill ~/.claude/skills/$skill; end
```

### pi / Gemini CLI / Codex / others

```bash
# bash/zsh
for skill in kagi-search kagi-extract; do ln -s "$(pwd)/$skill" ~/.agents/skills/"$skill"; done

# fish
for skill in kagi-search kagi-extract; ln -s (pwd)/$skill ~/.agents/skills/$skill; end
```

Each skill directory holds a `SKILL.md` that agents read, plus a wrapper script. The wrapper builds from source when Go 1.26+ is available, and otherwise offers a checksum-verified download from GitHub releases.

> Downloading a binary needs consent, and agents have no terminal to give it at. Either run the wrapper once from an interactive shell, install Go so it builds from source, or set `KAGI_SKILLS_ASSUME_YES=1`.

### Without cloning

```bash
go install github.com/Nighthawk42/kagi-skills/cmd/kagi-search@latest
go install github.com/Nighthawk42/kagi-skills/cmd/kagi-extract@latest
```

## Examples

```bash
# Search
kagi-search "go 1.27 release notes" -n 5

# Search and read the top result in one request
kagi-search "attention is all you need" --extract 1

# Scope to specific documentation sites
kagi-search "context cancellation" --site go.dev,pkg.go.dev --json

# Recent news
kagi-search "kubernetes release" --news --since week

# Academic sources, via a built-in Kagi lens
kagi-search "monad transformers" --lens 2

# Just the small-web index
kagi-search "self hosting" --only interesting_finds

# Read specific pages
kagi-extract https://go.dev/doc/go1.27 https://go.dev/blog/go1.27

# Chain the two
kagi-search "rust async book" --json \
  | jq -r '.collections.search[].url' | head -5 | kagi-extract
```

## Development

Single Go module, no dependencies.

```
cmd/kagi-search/     kagi-search entry point
cmd/kagi-extract/    kagi-extract entry point
internal/kagi/       Kagi API v1 client
internal/cli/        shared argument parsing
kagi-search/         SKILL.md + wrapper
kagi-extract/        SKILL.md + wrapper
```

| Target | Description |
| --- | --- |
| `make build` | Build both binaries into each skill's `.bin/` |
| `make test` | `go test ./...` |
| `make test-race` | `go test -race ./...` |
| `make vet` | `go vet ./...` |
| `make lint` | golangci-lint |
| `make fmt` | gofumpt |
| `make check` | format check, vet, test and lint |
| `make clean` | Remove built binaries and `dist/` |

Set `KAGI_API_HOST` to point the client at a stub server; the test suite uses this and never touches the network.

## Removed tools

Up to v1.1.0 this project also wrapped three Kagi APIs that exist only in v0: **FastGPT**, **Universal Summarizer** and **Enrichment** (Teclis / TinyGem).

They were removed in v2.0.0 because they are unreachable for new users. Kagi issues v0 and v1 credentials separately, a v1 key returns `401` on every v0 endpoint, and Kagi has not ported any of the three to v1 — `/api/v1/summarize` answers `404 general.endpoint_not_found`. Kagi's docs say the summarizer is "not yet available in v1".

If you hold a v0 token with remaining credit, the last version containing those tools is [v1.1.0](https://github.com/Nighthawk42/kagi-skills/tree/v1.1.0).

Some of what Enrichment offered is reachable from v1 search: `--only interesting_finds` returns Kagi's small-web index, and `--lens 107` scopes a whole query to it.

## Credits

A fork of [joelazar/kagi-skills](https://github.com/joelazar/kagi-skills) (archived), rewritten for the Kagi API v1. MIT licensed — see [LICENSE](LICENSE).
