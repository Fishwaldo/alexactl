# alexactl — maintenance phase

Go port of the Python library **aioamazondevices**
(https://github.com/chemelli74/aioamazondevices), API frozen at v1.0.0.
The porting-phase guide lives at `docs/history/CLAUDE-porting-phase.md`
and may still help when reading older commits.

## Architecture (one screen)

`alexactl.Client` is the entry point; sub-services hang off it:
`Auth` (OAuth + form-scrape login, OTP), `Devices` (list/refresh/inspect,
typed-controller surface on `*Device`), `Media` (Speak/Announce/SetVolume
+ Sequence facade), `Sensors` (state enrichment, optimistic local
updates), `Push` (HTTP/2 multipart event stream). All public methods take
`ctx` first and return `error` last; `*Device` is read-only from caller
perspective. Internal HTTP wrapping (`internal/httpwrapper`), GraphQL
codegen (`internal/alexagraphql` via `genqlient`), and credential
scrubbing (`internal/scrub`) are sealed under `internal/` and not part of
the v1.0.0 API surface. Typed enums (`internal/amzconst` for constants +
root-package `enums_enum.go` for `go-enum` generated string enums) carry
all wire-vocabulary.

## Upstream tracking workflow

1. Watch upstream tags at https://github.com/chemelli74/aioamazondevices/tags.
2. Diff `<last-ported-tag>..<new-tag>` in the upstream repo.
3. Map changed Python files → Go files (`media.py` ↔ `media.go`,
   `sensors.py` ↔ `sensors.go`, etc.); port the delta.
4. Bump `UpstreamVersion` in `version.go`, add a CHANGELOG entry under
   `[Unreleased]` mentioning the upstream tag + commit hash ported from.
5. Cut a Go semver release independent of upstream's numbering — additive
   ports go in `MINOR`, bugfixes in `PATCH`, signature changes in `MAJOR`.

## Adding a new push event type

1. Add the wire constant to `internal/amzconst` (e.g. `PushFooBar`).
2. Add a typed struct to `push_events.go` implementing the `PushEvent`
   interface; embed `pushEventBase` for `Type()` / `RawJSON()`.
3. Add the decoder case in `push_decode.go` keyed on the wire constant.
4. Add a fixture under `testdata/push/foo-bar.json` (scrubbed from a real
   capture) + a decode round-trip test in `push_decode_test.go`.
5. Document the new event in `CHANGELOG.md` under `### Added`.

## Commit message format

Conventional Commits: `type(scope): summary` — imperative mood, lowercase
summary, no trailing period, ≤72 chars. Append `!` before the colon for a
breaking change (e.g. `feat(cli)!: drop --serial flag`).

- **Types:** `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `ci`,
  `style`, `port` (an upstream-delta port). Scope is the area touched
  (`media`, `devices`, `push`, `cli`, `cache`, `httpwrapper`, …) and is
  optional for repo-wide changes.
- Body (optional, after a blank line) explains the *what* and *why*,
  wrapped at ~72 cols; let the diff show the *how*.
- **No trailers.** Never add `Co-Authored-By`, `Signed-off-by`, or any
  AI-attribution line.
- A user-visible change lands its CHANGELOG entry in the same commit.

## Release process

1. Promote `[Unreleased]` to `[X.Y.Z] - YYYY-MM-DD` in `CHANGELOG.md`.
2. `go test -race ./...` + `golangci-lint run` clean.
3. Single commit `docs: promote [Unreleased] to [X.Y.Z]`.
4. `git tag -a vX.Y.Z -m "vX.Y.Z — <one-line summary>"`.
5. `git push origin main && git push origin vX.Y.Z` (only after explicit
   sign-off — never push tags without confirmation).
6. Goreleaser-driven binary builds for the test CLI run on tag push.

## CI expectations

- `go test -race ./...` and `golangci-lint run` must pass on every PR.
- CI matrix: Linux + macOS today; Windows planned for v1.1.
- No test may require real Amazon credentials. Use `httptest` + fixtures
  under `testdata/`. Live-capture HARs are `.gitignore`d (contain session
  cookies).

## Common contributor mistakes

- Forgetting `ctx context.Context` as the first parameter of a public
  method. Every public method takes `ctx`.
- Logging via `slog.Default()` from library code instead of
  `s.wrapper.Logger(ctx)`. Callers wire their own logger via
  `WithLogger`; library code must not write to a different sink.
- Returning `nil` after catching an error. Wrap with `%w` and propagate;
  let the caller decide.
- Adding deps for JSON acceleration, OAuth2, event-bus, or runtime enum
  libraries. All four are covered by stdlib + the existing dep set.
- Mutating `*Device` and expecting it to back-propagate to Amazon. Device
  is a read-only snapshot; mutations happen through the typed-controller
  accessors (`d.Power().On(ctx)`, `d.Brightness().Set(ctx, 80)`, etc.).
- Tests that hit live Amazon endpoints. Always `httptest` + `testdata/`
  fixtures.
