// Package alexagraphql contains the auto-generated typed GraphQL client
// used by alexactl to talk to alexa.amazon.com's
// /nexus/v1/graphql endpoint.
//
// The generated client (generated.go) is produced by genqlient from
// queries/*.graphql against a vendored introspection of the Alexa
// GraphQL schema (see genqlient.yaml for the schema path). Run
// `go generate ./internal/alexagraphql/...` to regenerate after editing
// any .graphql file.
//
// The Doer adapter (doer.go) routes generated-client requests through
// internal/httpwrapper so auth refresh, retry, scrub, and body logging
// behave the same as every other HTTP call in this library.
//
// Service-layer code in the root package translates generated response
// types into the public Device / DeviceSensor / ... shapes. No
// generated symbols are exposed via the public API.
package alexagraphql
