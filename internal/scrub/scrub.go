// Package scrub redacts sensitive fields from log payloads before they are
// emitted via slog. Field list is the Go port of aioamazondevices'
// utils.scrub_fields TO_REDACT set.
package scrub

import "strings"

// Replacement is the literal string substituted into redacted fields.
const Replacement = "[REDACTED]"

// SensitiveKeys lists JSON keys whose values are redacted by Fields. Lookups
// are case-insensitive and separator-insensitive: each key is stored
// lowercased with "_" and "-" stripped, and callers fold their lookup key the
// same way (see normalizeKey) before checking membership. This catches
// variants like "Authorization", "AccessToken", "Access-Token", "accessToken",
// "PASSWORD", "Set-Cookie", and "set_cookie" without enumerating every casing
// or separator style.
//
// Categories covered:
//   - Long-lived credentials: access_token, refresh_token, adp_token,
//     device_private_key, password, source_token, token, mac_dms, bearer,
//     authorization
//   - OAuth/login-flow secrets: otp, otpcode, code_verifier, authorization_code
//   - Cookie material: cookie, set-cookie, csrf, store_authentication_cookie,
//     website_cookies
//
// PII fields like name/address/customerId are NOT redacted — log output for
// this library is expected to be operator-controlled, and over-redaction makes
// debugging harder. Callers concerned about PII in logs should configure their
// slog handler to drop or transform fields at the handler level rather than
// relying on this package.
var SensitiveKeys = map[string]struct{}{
	"accesstoken":               {},
	"adptoken":                  {},
	"authorization":             {},
	"authorizationcode":         {},
	"bearer":                    {},
	"codeverifier":              {},
	"cookie":                    {},
	"csrf":                      {},
	"deviceprivatekey":          {},
	"macdms":                    {},
	"otp":                       {},
	"otpcode":                   {},
	"password":                  {},
	"refreshtoken":              {},
	"setcookie":                 {},
	"sourcetoken":               {},
	"storeauthenticationcookie": {},
	"token":                     {},
	"websitecookies":            {},
}

// normalizeKey folds a key for case- and separator-insensitive lookup
// against SensitiveKeys. It lowercases the key and removes "_" and "-".
func normalizeKey(key string) string {
	lower := strings.ToLower(key)
	if !strings.ContainsAny(lower, "_-") {
		return lower
	}
	b := make([]byte, 0, len(lower))
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if c == '_' || c == '-' {
			continue
		}
		b = append(b, c)
	}
	return string(b)
}

// isSensitive reports whether key (case- and separator-insensitively) names
// a credential.
func isSensitive(key string) bool {
	_, ok := SensitiveKeys[normalizeKey(key)]
	return ok
}

// Fields returns a deep copy of v with sensitive map keys redacted.
// Lists, scalars, and unknown types pass through. Key matching is
// case-insensitive (see SensitiveKeys).
func Fields(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if isSensitive(k) {
				out[k] = Replacement
			} else {
				out[k] = Fields(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = Fields(item)
		}
		return out
	default:
		return v
	}
}
