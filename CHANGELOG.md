# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v2.0.0] - 2026-11-04

Rewritten for the **Kagi API v1**. This fork takes over from the archived
[joelazar/kagi-skills](https://github.com/joelazar/kagi-skills), whose last
release predates v1 and targeted endpoints Kagi now calls deprecated.

### Added
- `kagi-extract`: a new skill wrapping Kagi's **Extract API**, turning up to 10
  URLs into clean markdown in a single concurrent request
- `kagi-search` now exposes the v1 feature set: workflows (`--news`,
  `--images`, `--videos`, `--podcasts`), Kagi **lenses** (`--lens`), domain and
  keyword filters, `--file-type`, date filters (`--since`, `--after`,
  `--before`), `--region`, `--page`, `--unsafe` and `--api-timeout`
- `--extract N` fetches full page markdown for the top N results inline, in the
  same request as the search
- Result categories the official Kagi MCP server discards are now surfaced,
  including `infobox`, `related_search` and **`interesting_finds`** (Kagi's
  small-web index). `--only` filters to the categories you want
- Automatic retry with exponential backoff on `429` and `5xx`, honouring
  `Retry-After`; configurable via `KAGI_MAX_RETRIES`
- `--meta` prints the API trace id, for use in Kagi support reports
- `SIGINT`/`SIGTERM` now cancel an in-flight request instead of being ignored
- A test suite: 64 test functions across the API client, argument parser and
  text handling. The project previously had none, and CI had no test job
- CI builds and tests on Linux, macOS and Windows, with `go test -race`
- A release step that asserts a built binary reports the right version, so a
  broken `-ldflags` path can no longer ship binaries claiming to be `dev`

### Changed
- **Search moved from the v0 API to v1**: `POST /api/v1/search` with
  `Authorization: Bearer`, replacing `GET /api/v0/search` with
  `Authorization: Bot`. **A v1 API key from <https://kagi.com/api/keys> is now
  required**; v0 keys return `401`
- Restructured from four single-file modules into one module with `cmd/` and
  `internal/`, removing roughly 400 lines duplicated across the old binaries
- Titles and snippets are now stripped of HTML entities and highlight markup
  (`&#39;`, `<strong>`, and the full anchors Kagi embeds in infoboxes). The v0
  code passed these through raw. Inline-extracted markdown is deliberately left
  untouched, so code samples such as `Box<dyn Future<Output = ()>>` survive
- Page content now comes from Kagi's Extract API rather than being scraped
  locally, which removed the only dependency, the regex HTML stripper, the
  SSRF-protection layer and a spoofed Chrome user agent
- `--limit` now accepts the documented range of 1–1024, up from a hardcoded cap
  of 100
- The module path is now `github.com/Nighthawk42/kagi-skills`, so `go install`
  and the wrapper scripts fetch from this fork rather than upstream
- README pricing corrected: search is **$12 / 1,000 queries**, not the $25 /
  1,000 the v0 API charged
- Go version requirement stays at 1.26; the module has **zero dependencies**

### Removed
- **`kagi-fastgpt`, `kagi-summarizer` and `kagi-enrich`.** These wrap v0-only
  APIs with no v1 equivalent — `/api/v1/summarize` returns
  `404 general.endpoint_not_found` — and a v1 key is rejected by all of them, so
  they are unusable for anyone starting today. The last version containing them
  is [v1.1.0](https://github.com/Nighthawk42/kagi-skills/tree/v1.1.0).
  `--only interesting_finds` and `--lens 107` cover part of what Enrichment did
- **The `balance` command**, along with its on-disk cache. v0 returned
  `meta.api_balance` on every response; v1's `meta` is `{trace, node, ms}` and
  Kagi exposes no balance endpoint. Check <https://kagi.com/api/billing>
- `kagi-search content` — superseded by `kagi-extract`

### Fixed
- Wrapper scripts verified checksums with `shasum`, which is absent on most
  Linux distributions. Under `set -e` this aborted the install outright. They
  now try `sha256sum`, `shasum`, `openssl` and `certutil`, and refuse to
  install rather than skipping verification when none is found
- Wrapper scripts omitted the `.exe` suffix on Windows while the release
  workflow adds it, so every Windows download was a guaranteed 404
- Wrapper scripts prompted for download confirmation by reading `/dev/tty`,
  which fails under `set -e` in exactly the non-interactive agent context these
  skills are built for. They now detect it and explain the options, or proceed
  when `KAGI_SKILLS_ASSUME_YES=1`
- Wrapper scripts reported "neither curl nor wget found" when the real problem
  was an unreachable or release-less repository
- Wrapper scripts now resolve symlinks, so a skill symlinked into an agent's
  skills directory still finds the module root to build from
- Downloads land in a temporary file and are moved into place only after the
  checksum matches, so a failed install cannot leave a broken binary behind
- `--timeout` was parsed with `fmt.Sscanf("%d")`, which silently accepted
  `--timeout 12abc` as `12`. All numeric flags now use `strconv` and are
  range-checked, and `NaN` is rejected explicitly
- `--engine daphne` was accepted by the old summarizer while being absent from
  its help text and its own error message
- Related searches were sorted alphabetically, discarding Kagi's ranking
- `--content` fetched pages serially, each with the full client timeout, so ten
  results could take 150 seconds. Kagi now fetches concurrently, server-side
- Conflicting options are rejected rather than silently ignored: Kagi drops
  `lens_id` when inline lens options are present, which previously discarded
  the user's filters without a word

### Security
- Page fetching no longer happens locally, which removes the SSRF surface
  entirely rather than defending it
- The API key is sent only to `kagi.com`, and a `401` now explains the v0/v1
  key distinction instead of printing a bare "Unauthorized"

## [v1.1.0] - 2026-02-24

### Added
- Dedicated `balance` command across all four kagi skills to check Kagi API credit balance
- `AGENTS.md` with multi-module monorepo layout and Makefile task reference

### Security
- Wrapper scripts now verify SHA-256 checksums after downloading binaries

## [v1.0.0] - 2026-02-21

### Added
- Four CLI tools for Kagi APIs: `kagi-search`, `kagi-fastgpt`, `kagi-summarizer`, and `kagi-enrich`
- Pre-built binaries for Linux (amd64/arm64), macOS (amd64/arm64), and Windows (amd64)
- Auto-download wrapper scripts in each `SKILL.md` for seamless agent integration
- `go install` support via canonical module paths
- `--version` / `-v` flag for all binaries
- CI workflow (lint, format, build) and automated release workflow
- `Makefile` with `build`, `lint`, `test`, `fmt`, and `clean` targets
- MIT license

[Unreleased]: https://github.com/Nighthawk42/kagi-skills/compare/v2.0.0...HEAD
[v2.0.0]: https://github.com/Nighthawk42/kagi-skills/compare/v1.1.0...v2.0.0
[v1.1.0]: https://github.com/joelazar/kagi-skills/compare/v1.0.0...v1.1.0
[v1.0.0]: https://github.com/joelazar/kagi-skills/releases/tag/v1.0.0
