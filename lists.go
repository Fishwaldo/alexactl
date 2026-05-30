package amazondevices

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/cache"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// ListsService exposes the legacy /alexashoppinglists/api/ endpoints on
// www.amazon.<tld>. Surfaces both the default unnamed SHOPPING_LIST and
// any user-named custom LIST entries with full item CRUD.
//
// AUTH: cookie-based — the session restorer already populates the
// cookie jar with at-main / sess-at-main / session-id / ubid-main /
// x-main scoped to .amazon.<tld>, so requests to www.amazon.<tld>
// pick them up transparently.
//
// NO CACHING: every Get(ctx) is a fresh fetch. Callers cache if needed.
//
// ENDPOINT STABILITY WARNING: the /alexashoppinglists/api/ endpoints
// are undocumented and have survived multiple Lists-skill backend
// migrations (last seen working May 2026). Amazon could deprecate
// them without notice. Treat this service as best-effort; use the
// ItemChangeEvent push notification (PUSH_LIST_ITEM_CHANGE) as the
// canonical change signal.
//
// SCOPE: items added via the new Alexa+ web UI ("Metis") may not
// appear here — that UI uses a separate conversational data store.
// Items added by voice (Echo devices), the iOS Alexa app, or the
// legacy web UI WILL appear.
//
// Safe for concurrent use.
type ListsService struct {
	wrapper *httpwrapper.Wrapper

	mu      sync.RWMutex
	session *Session

	// itemsCache caches Get() for the Short-class TTL. Item-mutation
	// methods on *List/*Item invalidate it; callers needing strictly-fresh
	// data invoke Invalidate() before Get().
	itemsCache *cache.Cache[[]*List]
}

// setSession is called by AuthService.broadcast on Login/Restore.
func (s *ListsService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
}

// ListType is the typed enum that distinguishes the default shopping
// list (SHOPPING_LIST) from user-named custom lists (LIST).
// Generated values live in enums_enum.go: ListTypeShopping, ListTypeCustom.

// List is one logical list — either the default shopping list or a
// user-named custom list. Items are loaded with the parent on Get;
// the slice is sorted by (Completed, -CreatedAt) so active items
// show first.
type List struct {
	ID        string // base64-encoded amzn1.account.<id>-SHOPPING_ITEM
	Name      string // empty for the default shopping list
	Type      ListType
	Default   bool // true for the SHOPPING_LIST with defaultList:true
	Items     []*ListItem
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int

	service *ListsService // unexported back-ref for typed methods
}

// ListItem is the user-meaningful subset of the server's listItem dict.
// Fields hidden by design (encryptedValue, customerId, listItemMetadata)
// are accessible via ServerShape() for callers who need them.
type ListItem struct {
	ID        string
	Value     string
	Completed bool
	Category  string // "Other", "Dairy", "Baking & Cooking", ...
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int // optimistic-concurrency token, bumped server-side per Update

	list        *List          // back-ref for write methods
	serverShape map[string]any // raw response dict, echoed back on Update/Delete
}

// ServerShape returns a top-level copy of the raw server-shape map for
// this item — the unfiltered JSON dict as Amazon returned it. Useful
// for callers who need fields ListItem hides (encryptedValue,
// customerId, full listItemMetadata audit trail).
//
// The returned map is a shallow (depth-1) copy. Overwriting or
// injecting top-level keys does NOT affect the item's internal state,
// but nested maps and slices (notably listItemMetadata) are SHARED
// references — callers must not mutate them.
func (i *ListItem) ServerShape() map[string]any {
	if i.serverShape == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(i.serverShape))
	for k, v := range i.serverShape {
		out[k] = v
	}
	return out
}

// Get fetches every list on the account (both the default
// SHOPPING_LIST and named LIST entries) in a single round-trip.
// Lists are sorted by (Type, Name); items within each list are
// sorted by (Completed, -CreatedAt) so active entries come first.
//
// Cached for the Short-class TTL. Callers needing strictly-fresh data
// either pass DisableCache(ClassShort) at construction or invoke
// Invalidate() before Get().
func (s *ListsService) Get(ctx context.Context) ([]*List, error) {
	got, _, err := s.itemsCache.Get(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make([]*List, len(got))
	copy(out, got)
	return out, nil
}

// Invalidate drops the cached lists snapshot. The next Get triggers a
// fresh fetch.
func (s *ListsService) Invalidate() {
	if s.itemsCache != nil {
		s.itemsCache.InvalidateAll()
	}
}

// fetchItems is the loader for lists.items.
func (s *ListsService) fetchItems(ctx context.Context) ([]*List, error) {
	url := fmt.Sprintf("https://www.amazon.%s%s",
		s.wrapper.Domain(), amzconst.URIListsGetItems)

	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodGet,
		URL:          url,
		ExtraHeaders: http.Header{"User-Agent": []string{amzconst.UserAgentAmazon}},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodGet,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	data, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotRetrieveData, err)
	}

	out := make([]*List, 0, len(data))
	for _, raw := range data {
		entry, _ := raw.(map[string]any)
		if entry == nil {
			continue
		}
		l := buildList(s, entry)
		if l == nil {
			continue
		}
		out = append(out, l)
	}

	sort.SliceStable(out, func(i, j int) bool {
		// Default (shopping) list first; remaining sorted by (Type, Name).
		if out[i].Default != out[j].Default {
			return out[i].Default
		}
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// buildList constructs a *List from one entry of the getlistitems
// response. Returns nil for entries missing the required listInfo
// block (defensive — the live shape always has it).
func buildList(s *ListsService, entry map[string]any) *List {
	info, _ := entry["listInfo"].(map[string]any)
	if info == nil {
		return nil
	}
	id, _ := info["listId"].(string)
	if id == "" {
		return nil
	}
	// Field names on the live wire: listName, listType, createAt,
	// updateAt (NO "DateTime" suffix on the list-level timestamps —
	// item-level ones do carry it). Timestamps are numeric unix-ms,
	// not ISO strings.
	name, _ := info["listName"].(string)
	typeStr, _ := info["listType"].(string)
	listType, _ := ParseListType(typeStr)
	def, _ := info["defaultList"].(bool)
	version := intFromAny(info["version"])
	createdAt := parseListTime(info["createAt"])
	updatedAt := parseListTime(info["updateAt"])

	l := &List{
		ID:        id,
		Name:      name,
		Type:      listType,
		Default:   def,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Version:   version,
		service:   s,
	}

	rawItems, _ := entry["listItems"].([]any)
	for _, ri := range rawItems {
		im, _ := ri.(map[string]any)
		if im == nil {
			continue
		}
		item := buildListItem(l, im)
		if item == nil {
			continue
		}
		l.Items = append(l.Items, item)
	}

	sort.SliceStable(l.Items, func(i, j int) bool {
		if l.Items[i].Completed != l.Items[j].Completed {
			return !l.Items[i].Completed
		}
		return l.Items[i].CreatedAt.After(l.Items[j].CreatedAt)
	})
	return l
}

// buildListItem constructs a *ListItem from a single listItems entry.
// Stores the raw entry as serverShape so write methods can echo it
// back for optimistic-concurrency checks.
func buildListItem(l *List, raw map[string]any) *ListItem {
	id, _ := raw["id"].(string)
	if id == "" {
		return nil
	}
	value, _ := raw["value"].(string)
	category, _ := raw["categoryValue"].(string)
	completed, _ := raw["completed"].(bool)
	version := intFromAny(raw["version"])
	createdAt := parseListTime(raw["createdDateTime"])
	updatedAt := parseListTime(raw["updatedDateTime"])

	return &ListItem{
		ID:          id,
		Value:       value,
		Completed:   completed,
		Category:    category,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		Version:     version,
		list:        l,
		serverShape: raw,
	}
}

// parseListTime parses timestamps from Amazon's lists API. The live
// wire shape uses numeric unix milliseconds (e.g. 1.779937670634e+12),
// but earlier captures had ISO-8601 strings — accept both so the
// parser is robust across server-side format changes. Returns zero
// time on unrecognised input (caller treats as "unknown").
func parseListTime(v any) time.Time {
	switch x := v.(type) {
	case float64:
		if x <= 0 {
			return time.Time{}
		}
		return time.UnixMilli(int64(x))
	case int64:
		if x <= 0 {
			return time.Time{}
		}
		return time.UnixMilli(x)
	case int:
		if x <= 0 {
			return time.Time{}
		}
		return time.UnixMilli(int64(x))
	case string:
		if x == "" {
			return time.Time{}
		}
		if t, err := time.Parse(time.RFC3339Nano, x); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, x); err == nil {
			return t
		}
	}
	return time.Time{}
}

// intFromAny coerces a JSON-decoded value (float64 or json.Number-equivalent)
// to an int. Returns 0 for unrecognised input.
func intFromAny(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	}
	return 0
}

// AddItem POSTs a new item to this list. Returns the server-assigned
// *ListItem (with its new ID, timestamps, version, and any
// auto-categorisation metadata). The returned item has its back-ref
// to this *List wired up and its serverShape cached for later
// Update / Delete calls.
//
// On success the new *ListItem is appended to list.Items in place.
func (l *List) AddItem(ctx context.Context, value string) (*ListItem, error) {
	url := fmt.Sprintf("https://www.amazon.%s%s/%s",
		l.service.wrapper.Domain(), amzconst.URIListsAddItem, l.ID)

	resp, err := l.service.wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodPost,
		URL:          url,
		JSONBody:     map[string]any{"value": value, "type": "TASK"},
		ExtraHeaders: http.Header{"User-Agent": []string{amzconst.UserAgentAmazon}},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodPost,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	raw, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotRetrieveData, err)
	}

	item := buildListItem(l, raw)
	if item == nil {
		return nil, fmt.Errorf("%w: addlistitem response missing id", ErrCannotRetrieveData)
	}
	l.Items = append(l.Items, item)
	return item, nil
}

// Complete marks the item completed. No-op + nil error if already
// completed. On success: item.Completed=true, item.Version bumped to
// the server's value, item.UpdatedAt rewritten, item.serverShape
// replaced with the response dict.
func (l *List) Complete(ctx context.Context, item *ListItem) error {
	if item == nil || item.list != l {
		id := ""
		if item != nil {
			id = item.ID
		}
		return fmt.Errorf("%w: item %q", ErrListItemNotFound, id)
	}
	if item.Completed {
		return nil
	}
	return l.updateItem(ctx, item, map[string]any{"completed": true})
}

// Uncomplete is the inverse of Complete.
func (l *List) Uncomplete(ctx context.Context, item *ListItem) error {
	if item == nil || item.list != l {
		id := ""
		if item != nil {
			id = item.ID
		}
		return fmt.Errorf("%w: item %q", ErrListItemNotFound, id)
	}
	if !item.Completed {
		return nil
	}
	return l.updateItem(ctx, item, map[string]any{"completed": false})
}

// updateItem builds the PUT echo body by cloning item.serverShape and
// overlaying overrides, sends it, and applies the response back to
// the in-memory item on success. Leaves item untouched on error.
func (l *List) updateItem(ctx context.Context, item *ListItem, overrides map[string]any) error {
	url := fmt.Sprintf("https://www.amazon.%s%s",
		l.service.wrapper.Domain(), amzconst.URIListsUpdateItem)

	body := make(map[string]any, len(item.serverShape))
	for k, v := range item.serverShape {
		body[k] = v
	}
	for k, v := range overrides {
		body[k] = v
	}

	resp, err := l.service.wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodPut,
		URL:          url,
		JSONBody:     body,
		ExtraHeaders: http.Header{"User-Agent": []string{amzconst.UserAgentAmazon}},
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodPut,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	raw, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotRetrieveData, err)
	}

	if v, ok := raw["completed"].(bool); ok {
		item.Completed = v
	}
	if v, ok := raw["value"].(string); ok {
		item.Value = v
	}
	if v := intFromAny(raw["version"]); v != 0 {
		item.Version = v
	}
	if t := parseListTime(raw["updatedDateTime"]); !t.IsZero() {
		item.UpdatedAt = t
	}
	item.serverShape = raw
	return nil
}

// Delete removes the item from the list both server-side and from
// list.Items locally on success. Returns ErrListItemNotFound if the
// item is nil or belongs to a different list.
func (l *List) Delete(ctx context.Context, item *ListItem) error {
	if item == nil || item.list != l {
		id := ""
		if item != nil {
			id = item.ID
		}
		return fmt.Errorf("%w: item %q", ErrListItemNotFound, id)
	}

	url := fmt.Sprintf("https://www.amazon.%s%s",
		l.service.wrapper.Domain(), amzconst.URIListsDeleteItem)

	body := make(map[string]any, len(item.serverShape))
	for k, v := range item.serverShape {
		body[k] = v
	}

	resp, err := l.service.wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodDelete,
		URL:          url,
		JSONBody:     body,
		ExtraHeaders: http.Header{"User-Agent": []string{amzconst.UserAgentAmazon}},
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodDelete,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}

	for i, it := range l.Items {
		if it == item {
			l.Items = append(l.Items[:i], l.Items[i+1:]...)
			break
		}
	}
	return nil
}

// Rename rewrites the item's Value to newValue. On success:
// item.Value rewritten, item.Version bumped, item.UpdatedAt
// rewritten, item.serverShape refreshed.
func (l *List) Rename(ctx context.Context, item *ListItem, newValue string) error {
	if item == nil || item.list != l {
		id := ""
		if item != nil {
			id = item.ID
		}
		return fmt.Errorf("%w: item %q", ErrListItemNotFound, id)
	}
	return l.updateItem(ctx, item, map[string]any{"value": newValue})
}
