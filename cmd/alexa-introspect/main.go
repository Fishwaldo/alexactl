// Command alexa-introspect runs the standard GraphQL introspection
// query against alexa.amazon.com/nexus/v1/graphql using a logged-in
// session loaded from session.json. The result is written to stdout
// as pretty-printed JSON.
//
// Usage:
//
//	go run ./cmd/alexa-introspect -session ./session.json > schema.json
//	diff -u docs/superpowers/research/2026-05-25-alexa-graphql-schema.json schema.json
//
// Use this to detect schema drift after a long gap, or before adding
// new operations that may depend on schema additions.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"time"

	"github.com/Fishwaldo/alexactl"
)

const introspectionQuery = `query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types { ...FullType }
    directives { name description locations args { ...InputValue } }
  }
}
fragment FullType on __Type {
  kind name description
  fields(includeDeprecated: true) {
    name description
    args { ...InputValue }
    type { ...TypeRef }
    isDeprecated deprecationReason
  }
  inputFields { ...InputValue }
  interfaces { ...TypeRef }
  enumValues(includeDeprecated: true) { name description isDeprecated deprecationReason }
  possibleTypes { ...TypeRef }
}
fragment InputValue on __InputValue { name description type { ...TypeRef } defaultValue }
fragment TypeRef on __Type {
  kind name
  ofType { kind name ofType { kind name ofType { kind name ofType { kind name ofType { kind name ofType { kind name ofType { kind name } } } } } } }
}`

func main() {
	sessionPath := flag.String("session", "./session.json", "path to session.json")
	flag.Parse()

	if err := run(*sessionPath); err != nil {
		fmt.Fprintln(os.Stderr, "alexa-introspect:", err)
		os.Exit(1)
	}
}

func run(sessionPath string) error {
	sess, err := loadSession(sessionPath)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	if sess.Domain == "" {
		return fmt.Errorf("session has no domain")
	}

	host := "alexa.amazon." + sess.Domain
	endpoint := "https://" + host + "/nexus/v1/graphql"

	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("cookie jar: %w", err)
	}
	cookieURL, _ := url.Parse("https://" + host)
	cookies := make([]*http.Cookie, 0, len(sess.WebsiteCookies))
	for name, value := range sess.WebsiteCookies {
		// #nosec G124 -- client-side request cookies replayed from a saved session; Secure/HttpOnly/SameSite are server-response attributes and do not apply.
		cookies = append(cookies, &http.Cookie{Name: name, Value: value})
	}
	jar.SetCookies(cookieURL, cookies)

	httpClient := &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
	}

	body := map[string]any{
		"operationName": "IntrospectionQuery",
		"variables":     map[string]any{},
		"query":         introspectionQuery,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal introspection query: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("introspection request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return fmt.Errorf("pretty-print response: %w", err)
	}
	if _, err := pretty.WriteTo(os.Stdout); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	return nil
}

// loadSession reads session.json (the same format cmd/alexactl
// writes) and returns it. We use alexactl.Session here only as a
// JSON shape — no client construction.
func loadSession(path string) (*alexactl.Session, error) {
	// #nosec G304 -- this dev tool reads a session file at a path supplied by the operator on the CLI.
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s alexactl.Session
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
