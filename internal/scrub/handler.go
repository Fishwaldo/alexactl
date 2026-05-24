package scrub

import (
	"context"
	"log/slog"
)

// Handler wraps inner with a slog handler that scrubs sensitive attribute
// values. Credential keys (see SensitiveKeys) get their values replaced with
// "[REDACTED]"; matching is case-insensitive so attributes like
// "Authorization" or "AccessToken" are caught alongside the lowercase forms.
// Map- and group-valued attributes are recursively scrubbed, including
// []map[string]any slices.
//
// Use this at the boundary between library logging and the user-supplied
// logger so call sites can log raw values without per-site redaction.
func Handler(inner slog.Handler) slog.Handler {
	return &handler{inner: inner}
}

type handler struct {
	inner slog.Handler
}

func (h *handler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(scrubAttr(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	scrubbed := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		scrubbed[i] = scrubAttr(a)
	}
	return &handler{inner: h.inner.WithAttrs(scrubbed)}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{inner: h.inner.WithGroup(name)}
}

func scrubAttr(a slog.Attr) slog.Attr {
	// Credential keys — value replaced. Case- and separator-insensitive.
	if isSensitive(a.Key) {
		return slog.String(a.Key, Replacement)
	}

	switch a.Value.Kind() {
	case slog.KindGroup:
		sub := a.Value.Group()
		scrubbed := make([]slog.Attr, len(sub))
		for i, s := range sub {
			scrubbed[i] = scrubAttr(s)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(scrubbed...)}

	case slog.KindAny:
		v := a.Value.Any()
		switch x := v.(type) {
		case map[string]any:
			return slog.Any(a.Key, Fields(x))
		case map[string]string:
			conv := make(map[string]any, len(x))
			for k, val := range x {
				conv[k] = val
			}
			return slog.Any(a.Key, Fields(conv))
		case []any:
			return slog.Any(a.Key, Fields(x))
		case []map[string]any:
			out := make([]any, len(x))
			for i, m := range x {
				out[i] = Fields(m)
			}
			return slog.Any(a.Key, out)
		}
	}

	return a
}
