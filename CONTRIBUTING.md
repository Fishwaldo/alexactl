# Contributing to alexactl

Thanks for your interest! This project is a Go library heavily inspired by
[aioamazondevices](https://github.com/chemelli74/aioamazondevices). It tracks
upstream releases for behavioral parity with Amazon's services.

## Prerequisites

- Go 1.24+ (`go version`)
- `golangci-lint` for local linting: `brew install golangci-lint` or
  https://golangci-lint.run/usage/install/
- `enumer` is tracked as a tool in `go.mod`; invoke via
  `go tool enumer ...` — no separate install needed.

## Workflow

1. Fork and create a feature branch.
2. Read `CLAUDE.md` for conventions.
3. Make focused commits using Conventional Commits (`feat:`, `fix:`,
   `port:`, `docs:`, `test:`, `chore:`, `refactor:`, `ci:`).
4. Run `go test -race ./...` and `golangci-lint run` before pushing.
5. Update `CHANGELOG.md` under "Unreleased".
6. Open a PR. For ports, link the upstream file at a specific commit hash.

## Testing

Unit and HTTP-handler tests run via `go test ./...`. No real Amazon
credentials are used. Capture fresh fixtures via the test CLI's
`--save-fixtures` flag, then scrub via the same `internal/scrub` pipeline
the logger uses.

## Code of Conduct

Be excellent to each other.
