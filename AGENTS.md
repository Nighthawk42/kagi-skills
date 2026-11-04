# AGENTS.md

Single Go module, **zero dependencies** (standard library only). Do not add a
dependency without a strong reason — staying dependency-free is a feature of
these tools, not an accident.

```
go.mod                 module github.com/Nighthawk42/kagi-skills (go 1.26)
cmd/kagi-search/       kagi-search entry point
cmd/kagi-extract/      kagi-extract entry point
internal/kagi/         Kagi API v1 client (client, search, extract, text)
internal/cli/          shared argument parsing and process plumbing
kagi-search/           SKILL.md + kagi-search.sh wrapper
kagi-extract/          SKILL.md + kagi-extract.sh wrapper
```

The skill directories hold only docs and a wrapper. Binaries are built into
`<skill-dir>/.bin/<name>`, which is where the wrapper looks for them.

## Tasks

| Target | Description |
| --- | --- |
| `make build` | Build both binaries into each skill's `.bin/` |
| `make test` | `go test ./...` |
| `make test-race` | `go test -race ./...` |
| `make vet` | `go vet ./...` |
| `make lint` | `golangci-lint run --config .golangci.yml ./...` |
| `make fmt` | `gofumpt -w -l .` |
| `make check` | format check, vet, test and lint |
| `make clean` | Remove built binaries and `dist/` |

`golangci-lint` and `gofumpt` are not vendored; install them separately.

## Things worth knowing before changing this code

**The API version matters.** These tools speak Kagi API **v1** only:
`POST https://kagi.com/api/v1/{search,extract}` with `Authorization: Bearer`.
The v0 API (`/api/v0/...`, `Authorization: Bot`) is deprecated, uses separately
issued credentials, and a v1 key gets `401` from it. v1 has exactly two
endpoints — there is no v1 summarize, fastgpt or enrich, so do not add skills
for them expecting a v1 key to work.

**Clean API text, but never extracted markdown.** Kagi embeds HTML entities and
markup in `title` and `snippet` (`&#39;`, `<strong>`, and full `<a href>`
anchors in infoboxes). Run those through `kagi.CleanText`. But when
`--extract` is in play the snippet is replaced by page *markdown*, and
stripping angle-bracket spans from it would corrupt code such as
`Box<dyn Future<Output = ()>>`. `isExtracted()` in `cmd/kagi-search` decides
which is which; keep that distinction intact.

**`limit` caps each category, not the response.** A `search` workflow can
return more rows in total than `--limit`, because the cap applies per
collection. Single-category workflows cap the total.

**`meta` has no balance.** v1 returns `{trace, node, ms}`. There is no balance
field and no balance endpoint, which is why there is no `balance` command.

**Per-page extract failures arrive with HTTP 200.** `ExtractResponse.Data[].Error`
is how a single page reports failure. Successful pages must still be written
out; only exit non-zero when every page failed.

**Error envelopes come in two shapes.** `/search` returns `{"error": [...]}`
(singular) and `/extract` and 404s return `{"errors": [...]}` (plural). The
client decodes both — see `errorEnvelope.details()`.

**Parse numbers with `strconv`, not `fmt.Sscanf`.** `Sscanf("%d")` accepts
`12abc` as `12` without error; this was a real bug in the code this replaced.
Use the `internal/cli` `Args` helpers, which also range-check and reject `NaN`.

## Testing

Tests are standard-library only and never touch the network. The client reads
its base URL from `KAGI_API_HOST`, so point it at an `httptest.NewServer`:

```go
t.Setenv("KAGI_API_HOST", server.URL)
```

Set `MaxRetries` on the `Client` struct directly to keep retry tests fast —
backoff starts at 500ms.

## Wrapper scripts

`kagi-search/kagi-search.sh` and `kagi-extract/kagi-extract.sh` are identical
apart from `BIN_NAME` and one comment. **Edit one and regenerate the other**:

```bash
sed -e 's/^BIN_NAME="kagi-search"$/BIN_NAME="kagi-extract"/' \
    -e 's/the kagi-search skill/the kagi-extract skill/' \
    kagi-search/kagi-search.sh > kagi-extract/kagi-extract.sh
chmod +x kagi-extract/kagi-extract.sh
```

They carry fixes that are easy to regress, so test these paths after changes:
symlinked skill directory, no Go installed, no published release, non-interactive
shell with no tty, and a Windows `uname` (the `.exe` suffix).

## Releases

Tagging `v*` triggers `.github/workflows/release.yml`. The version is injected
as `-X github.com/Nighthawk42/kagi-skills/internal/cli.Version=<tag>` — note
this is **not** `main.version`, and a wrong path silently ships binaries
reporting `dev`. A release step asserts the built binary prints the right
version, so that failure is caught.

Artifact names are `<binary>_<tag>_<goos>_<goarch>[.exe]` plus a
`checksums.txt`. The wrapper scripts construct those filenames exactly, so
renaming artifacts breaks installation for everyone.
