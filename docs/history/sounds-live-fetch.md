# Sounds: deliberate divergence from upstream's static table

**Decision date:** 2026-05-28
**Decision:** The Go port fetches the Alexa sound catalogue at runtime
via `/api/behaviors/entities?skillId=amzn1.ask.1p.sound`, replacing the
hardcoded `internal/amzconst/sounds.go` that was ported from upstream
`aioamazondevices` `const/sounds.py`.

**Why:** The list is now always-fresh, returns localized labels per
account, and exposes category metadata the flat `map[string]string` was
throwing away. `PlaySound` also rejects unknown IDs client-side.

**Upstream workflow:** `aioamazondevices` still ships a static table that
its maintainer manually refreshes by curling the same endpoint from a
logged-in browser and committing the JSON; the
`scripts/home-assistant/update_sounds_list.py` script only transforms
the committed file. The Go port deliberately skips that workflow.

**For a future porter:** when a future upstream tag rewrites
`const/sounds.py`, do NOT re-port the static map back into
`internal/amzconst/`. The Go side fetches live; bump
[`internal/amzconst/http.go`](../../internal/amzconst/http.go) only if
the endpoint URL itself changes.
