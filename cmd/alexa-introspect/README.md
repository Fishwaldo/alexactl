# alexa-introspect

Issues a GraphQL introspection query against
`alexa.amazon.com/nexus/v1/graphql` using a logged-in session, and writes
the response as JSON to stdout.

## Usage

After logging in via `cmd/alexactl login`, `session.json`
carries the cookie jar + tokens the introspection request needs. Use
a **fresh** session — this tool talks directly to Amazon via a stdlib
`*http.Client`, bypassing the library's auth-refresh path. If the
access token has expired, the request will 401 and the tool will not
retry; log in again to mint a new token.

```sh
go run ./cmd/alexa-introspect > /tmp/schema.json
diff -u docs/superpowers/research/2026-05-25-alexa-graphql-schema.json /tmp/schema.json
```

If the diff is non-empty, Amazon has shipped a schema change. Update the
vendored schema and regenerate the typed client:

```sh
cp /tmp/schema.json docs/superpowers/research/2026-05-25-alexa-graphql-schema.json
npx --yes graphql-introspection-json-to-sdl \
  docs/superpowers/research/2026-05-25-alexa-graphql-schema.json \
  > docs/superpowers/research/2026-05-25-alexa-graphql-schema.graphql
go generate ./internal/alexagraphql/...
go test ./...
```

Inspect the diff carefully before regenerating — additions are usually
safe; renames/removals will break generated code at compile time.
