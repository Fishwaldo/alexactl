# goamazondevices — porting phase

This repo is an in-progress Go port of the Python library **aioamazondevices**
(https://github.com/chemelli74/aioamazondevices). We are NOT vendoring or
submoduling the upstream source — read it directly from GitHub when porting,
then write idiomatic Go in this tree.

> A directory named `aioamazondevices/` may exist at the repo root locally
> during early porting work. It is a throwaway reference clone and MUST NOT
> be committed (it is in `.gitignore`). When porting later modules, prefer
> fetching from upstream directly so we don't drift from a stale local copy.

## Where we are

- Phase 1 (MVP, v0.1.0): auth, devices, Speak/Announce/SetVolume
- Phase 2 (v0.2.0): sensors, DnD, notifications, history, full media
- Phase 3 (v0.3.0): HTTP/2 push streaming
- Phase 4 (v1.0.0): API freeze, maintenance-mode CLAUDE.md replaces this one

Design spec: `docs/superpowers/specs/2026-05-24-goamazondevices-design.md`.
Phase 1 plan: `docs/superpowers/plans/2026-05-24-goamazondevices-phase1.md`.
Always re-read the spec before architectural changes.

## Mapping conventions (Python → Go)

- `class AmazonEchoApi` → `*amazondevices.Client` with sub-services
  (`Auth`, `Devices`, `Media`, and later `Sensors`, `Push`, ...).
- Each Python handler class → one Go service struct in its own file.
- `async def foo(...)` → `func (s *FooService) Foo(ctx context.Context, ...) (..., error)`.
  `ctx` is ALWAYS the first arg. Public methods return `error` last.
- Python `dict[str, Any]` for opaque JSON → `map[string]any`, but only
  internally. Public API uses typed structs.
- `aiosignal.Signal[...]` → `<-chan PushEvent`. No callback-list pattern.
- `asyncio.Task` / `TaskGroup` → goroutines + `context.Context` cancellation +
  `sync.WaitGroup`. Use `errgroup` only if we genuinely need error propagation.
- `aiohttp.ClientSession` / `httpx.AsyncClient` → stdlib `*http.Client`
  wrapped by `internal/httpwrapper`.
- `BeautifulSoup(...)` → `goquery.NewDocumentFromReader`.
- Python sentinel exceptions (e.g. `CannotAuthenticate`) → exported sentinel
  errors (`ErrCannotAuthenticate`). Wrap with `%w` for detail.
- `StrEnum` classes → `abice/go-enum`-generated typed string consts. Tool tracked via `tool github.com/abice/go-enum` in `go.mod`.

## Libraries (don't reinvent these)

| Need | Use |
|---|---|
| HTTP client | stdlib `net/http` |
| HTTP retries | `github.com/hashicorp/go-retryablehttp` |
| HTML scraping | `github.com/PuerkitoBio/goquery` |
| Multipart parsing (Phase 3) | stdlib `mime/multipart` |
| Backoff (non-HTTP loops) | `github.com/cenkalti/backoff/v5` |
| Country → language | `golang.org/x/text/language` |
| CLI | `github.com/spf13/cobra` + `viper` |
| Tests | `github.com/stretchr/testify` |
| Logging | stdlib `log/slog` |

DO NOT add deps for: JSON acceleration, OAuth2 (we scrape forms), event-bus
libraries, or runtime enum libraries.

## What NOT to do

- Don't add features that upstream aioamazondevices doesn't have. Open an
  issue first.
- Don't introduce global state, package-level mutable vars, or `init()`
  side effects.
- Don't catch errors and return `nil` — wrap with `%w` and propagate.
- Don't log via global `slog.Default()` from library code — always use the
  injected `*slog.Logger` from the Client.
- Don't write tests that require real Amazon credentials. Use `httptest` +
  fixtures under `testdata/`.
- Don't reorganize package layout without updating the design spec first.
- Don't manually call `scrub.Fields` at log sites. The `scrub.Handler` slog
  middleware (wired automatically inside `httpwrapper.New`) redacts
  credential-key values and recurses into map/group attrs. Just log raw values.
- The library logs via `slog.Default()` if no `WithLogger` is supplied.
  Callers wanting silence should supply `slog.New(slog.DiscardHandler)`
  (Go 1.24+).

## Workflow checklist for a new Python module port

1. Read the upstream Python file on GitHub at a specific commit hash.
2. Confirm the module is in the current phase per the spec.
3. Write or update fixtures under `testdata/<area>/` from a scrubbed real
   response if available.
4. Write Go in the matching file (e.g., `media.go` ports
   `implementation/media.py`).
5. Write tests first when practical (httptest server + fixtures).
6. Run `go test -race ./...` and `golangci-lint run`.
7. Update `CHANGELOG.md` under "Unreleased".
8. PR: title `port: <module>`; body links to upstream file + commit hash.
