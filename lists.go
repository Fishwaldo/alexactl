// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/cache"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// ListsService exposes the v2 /alexashoppinglists/api/v2/lists ToDo
// API on www.amazon.<tld>. Surfaces both the default unnamed SHOP list
// and any user-named TODO/CUSTOM list entries with full item CRUD.
//
// AUTH: cookie-based — the session restorer already populates the
// cookie jar with at-main / sess-at-main / session-id / ubid-main /
// x-main scoped to .amazon.<tld>, so requests to www.amazon.<tld>
// pick them up transparently.
//
// CACHING: Get(ctx) is served from the Short-class cache; item-mutation
// methods on *List invalidate it so the next Get triggers a fresh
// fetch.
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
	// methods on *List invalidate it; callers needing strictly-fresh
	// data invoke Invalidate() before Get().
	itemsCache *cache.Cache[[]*List]
}

// setSession is called by AuthService.broadcast on Login/Restore.
func (s *ListsService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
}

// site returns the retail host requests are built against
// (https://www.amazon.<tld>).
func (s *ListsService) site() string {
	return fmt.Sprintf("https://www.amazon.%s", s.wrapper.Domain())
}

// ListType is the typed enum that distinguishes the default shopping
// list (SHOP) from to-do (TODO) and user-named custom (CUSTOM) lists.
// Generated values live in enums_enum.go: ListTypeShop, ListTypeTodo,
// ListTypeCustom.

// List is one logical list on the v2 ToDo API. Items are loaded with
// the parent on Get; the slice is sorted active-first, then by Value.
type List struct {
	ID    string
	Name  string // empty for the default shopping list
	Type  ListType
	Items []*ListItem

	service *ListsService // unexported back-ref for typed methods
}

// ListItem is the user-meaningful subset of the server's v2 item
// dict.
type ListItem struct {
	ID        string
	Value     string
	Completed bool
	Version   int // optimistic-concurrency token, required by every write call

	list *List // back-ref for write methods
}

// Get fetches every list on the account (the default SHOP list and
// any named TODO/CUSTOM entries) along with their items. The default
// shopping list (Type == ListTypeShop) sorts first; remaining lists
// are sorted by (Type, Name). Items within each list are sorted
// active-first, then by Value.
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

// fetchItems is the loader for lists.items: fetch the list of lists,
// then fetch each list's items.
func (s *ListsService) fetchItems(ctx context.Context) ([]*List, error) {
	url := s.site() + amzconst.URIToDoBase + "/fetch"

	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodPost,
		URL:          url,
		JSONBody:     map[string]any{},
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
	data, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotRetrieveData, err)
	}

	rawLists, _ := data["listInfoList"].([]any)
	out := make([]*List, 0, len(rawLists))
	for _, ri := range rawLists {
		entry, _ := ri.(map[string]any)
		if entry == nil {
			continue
		}
		l := buildList(s, entry)
		if l == nil {
			continue
		}
		out = append(out, l)
	}

	for _, l := range out {
		items, ferr := s.fetchListItems(ctx, l)
		if ferr != nil {
			return nil, ferr
		}
		l.Items = items
	}

	sort.SliceStable(out, func(i, j int) bool {
		// Default (shopping) list first; remaining sorted by (Type, Name).
		iShop, jShop := out[i].Type == ListTypeShop, out[j].Type == ListTypeShop
		if iShop != jShop {
			return iShop
		}
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// fetchListItems fetches every item on a single list via the v2
// items/fetch endpoint.
func (s *ListsService) fetchListItems(ctx context.Context, l *List) ([]*ListItem, error) {
	url := fmt.Sprintf("%s%s/%s/items/fetch?limit=100", s.site(), amzconst.URIToDoBase, l.ID)

	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodPost,
		URL:          url,
		JSONBody:     map[string]any{},
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
	data, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotRetrieveData, err)
	}

	rawItems, _ := data["itemInfoList"].([]any)
	items := make([]*ListItem, 0, len(rawItems))
	for _, ri := range rawItems {
		im, _ := ri.(map[string]any)
		if im == nil {
			continue
		}
		item := buildListItem(l, im)
		if item == nil {
			continue
		}
		items = append(items, item)
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Completed != items[j].Completed {
			return !items[i].Completed
		}
		return items[i].Value < items[j].Value
	})
	return items, nil
}

// buildList constructs a *List from one entry of the v2 listInfoList
// response. Returns nil for entries missing a listId (defensive — the
// live shape always has it).
func buildList(s *ListsService, entry map[string]any) *List {
	id, _ := entry["listId"].(string)
	if id == "" {
		return nil
	}
	name, _ := entry["listName"].(string)
	typeStr, _ := entry["listType"].(string)
	listType, _ := ParseListType(typeStr)

	return &List{
		ID:      id,
		Name:    name,
		Type:    listType,
		service: s,
	}
}

// buildListItem constructs a *ListItem from a single v2 itemInfoList
// entry. Returns nil for entries missing an itemId (defensive).
func buildListItem(l *List, raw map[string]any) *ListItem {
	id, _ := raw["itemId"].(string)
	if id == "" {
		return nil
	}
	name, _ := raw["itemName"].(string)
	statusStr, _ := raw["itemStatus"].(string)
	status, _ := ParseListItemStatus(statusStr)
	version := intFromAny(raw["version"])

	return &ListItem{
		ID:        id,
		Value:     name,
		Completed: status == ListItemStatusComplete,
		Version:   version,
		list:      l,
	}
}

// parseListTime parses timestamps from Amazon's schedules/lists APIs.
// Accepts both numeric unix milliseconds and ISO-8601 strings, since
// different endpoints have used different formats. Returns zero time
// on unrecognised input (caller treats as "unknown").
//
// Shared by alarms.go, reminders.go, and timers.go — not used by
// ListsService itself (the v2 ToDo API doesn't return timestamps for
// lists/items), but kept here as the historical home of this helper.
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

// AddItem POSTs a new item to this list as a KEYWORD item. On success
// the v2 API echoes the newly-created item nested under itemInfoList;
// the returned *ListItem is built from itemInfoList[0] so it carries
// the server-assigned itemId, version, and status. The cache is
// invalidated so the next Get(ctx) reconciles with the server's
// canonical state.
//
// If the add succeeds (2xx) but the response omits itemInfoList,
// AddItem returns ErrCannotRetrieveData rather than a zero-ID item
// (which write methods could misuse); the item was created server-side
// and a fresh Get(ctx) will surface it.
//
// On success the new *ListItem is appended to list.Items in place.
func (l *List) AddItem(ctx context.Context, value string) (*ListItem, error) {
	url := fmt.Sprintf("%s%s/%s/items", l.service.site(), amzconst.URIToDoBase, l.ID)

	resp, err := l.service.wrapper.Do(ctx, httpwrapper.Request{
		Method: http.MethodPost,
		URL:    url,
		JSONBody: map[string]any{
			"items": []map[string]any{
				{"itemType": "KEYWORD", "itemName": value},
			},
		},
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

	// Invalidate up-front: the item was created server-side regardless
	// of whether we can parse the echo, so the cache is now stale.
	l.service.Invalidate()

	raw, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotRetrieveData, err)
	}
	// The created item is nested under itemInfoList (confirmed against
	// live Amazon 2026-07-12).
	infos, _ := raw["itemInfoList"].([]any)
	if len(infos) == 0 {
		return nil, fmt.Errorf("%w: add succeeded but response missing itemInfoList", ErrCannotRetrieveData)
	}
	first, _ := infos[0].(map[string]any)
	item := buildListItem(l, first)
	if item == nil {
		return nil, fmt.Errorf("%w: add succeeded but itemInfoList entry missing itemId", ErrCannotRetrieveData)
	}
	l.Items = append(l.Items, item)
	return item, nil
}

// Complete marks the item completed. No-op + nil error if already
// completed. On success: item.Completed=true, item.Version bumped to
// the server's value from the itemInfo response.
func (l *List) Complete(ctx context.Context, item *ListItem) error {
	if item == nil || item.list != l {
		return fmt.Errorf("%w: item %q", ErrListItemNotFound, itemID(item))
	}
	if item.Completed {
		return nil
	}
	if err := l.updateItemAttribute(ctx, item, "itemStatus", string(ListItemStatusComplete)); err != nil {
		return err
	}
	item.Completed = true
	return nil
}

// Uncomplete is the inverse of Complete.
func (l *List) Uncomplete(ctx context.Context, item *ListItem) error {
	if item == nil || item.list != l {
		return fmt.Errorf("%w: item %q", ErrListItemNotFound, itemID(item))
	}
	if !item.Completed {
		return nil
	}
	if err := l.updateItemAttribute(ctx, item, "itemStatus", string(ListItemStatusActive)); err != nil {
		return err
	}
	item.Completed = false
	return nil
}

// Rename rewrites the item's Value to newValue via an itemName
// attribute update. On success: item.Value rewritten, item.Version
// bumped from the itemInfo response.
func (l *List) Rename(ctx context.Context, item *ListItem, newValue string) error {
	if item == nil || item.list != l {
		return fmt.Errorf("%w: item %q", ErrListItemNotFound, itemID(item))
	}
	if err := l.updateItemAttribute(ctx, item, "itemName", newValue); err != nil {
		return err
	}
	item.Value = newValue
	return nil
}

// updateItemAttribute PUTs a single itemAttributesToUpdate entry for
// item, keyed by its current Version for optimistic concurrency
// (.../items/{id}?version={item.Version}). On a 2xx it parses the
// response's singular "itemInfo" object and bumps item.Version from it
// so a caller holding the item across a subsequent write (e.g.
// Complete then Delete) sends the correct bumped version. Callers
// apply the field-specific state (Completed / Value) themselves.
//
// The itemInfo response shape was confirmed against live Amazon
// 2026-07-12: PUT returns {"itemInfo":{"itemId":..,"version":<bumped>,
// "itemStatus":..,"itemName":..}}.
func (l *List) updateItemAttribute(ctx context.Context, item *ListItem, attrType, value string) error {
	url := fmt.Sprintf("%s%s/%s/items/%s?version=%d",
		l.service.site(), amzconst.URIToDoBase, l.ID, item.ID, item.Version)

	body := map[string]any{
		"itemAttributesToUpdate": []map[string]any{
			{"type": attrType, "value": value},
		},
		"itemAttributesToRemove": []string{},
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

	if raw, derr := httpwrapper.DecodeJSON(resp); derr == nil {
		if info, ok := raw["itemInfo"].(map[string]any); ok {
			if v := intFromAny(info["version"]); v != 0 {
				item.Version = v
			}
		}
	}
	return nil
}

// Delete removes the item from the list both server-side and from
// list.Items locally on success. Returns ErrListItemNotFound if the
// item is nil or belongs to a different list.
func (l *List) Delete(ctx context.Context, item *ListItem) error {
	if item == nil || item.list != l {
		return fmt.Errorf("%w: item %q", ErrListItemNotFound, itemID(item))
	}

	url := fmt.Sprintf("%s%s/%s/items/%s?version=%d",
		l.service.site(), amzconst.URIToDoBase, l.ID, item.ID, item.Version)

	resp, err := l.service.wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodDelete,
		URL:          url,
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

// itemID safely extracts an item's ID for error messages, tolerating nil.
func itemID(item *ListItem) string {
	if item == nil {
		return ""
	}
	return item.ID
}
