// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newListsTestClient mirrors newNotificationsTestClient. Uses the
// hostRewriter Transport so URLs built from wrapper.Domain() are
// rewritten to the test server.
func newListsTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	sess := &Session{
		AccountCustomerID: "amzn1.account.TEST",
		Site:              "https://www.amazon.com",
		Domain:            "com",
		DeviceInfo: DeviceInfo{
			DeviceSerialNumber: "TEST-SERIAL",
			DeviceType:         "TEST-TYPE",
		},
	}
	hc := &http.Client{Transport: &hostRewriter{base: srv.URL, inner: srv.Client().Transport}}
	c, err := NewClient(
		WithHTTPClient(hc),
		WithSite("https://www.amazon.com"),
		WithSession(sess),
	)
	require.NoError(t, err)
	return c
}

// v2ListsHandler routes the v2 ToDo API surface used by ListsService:
// POST .../v2/lists/fetch (lists), POST .../v2/lists/{id}/items/fetch
// (items), POST .../v2/lists/{id}/items (add), PUT/DELETE
// .../v2/lists/{id}/items/{itemId} (mutate). addBody/putFunc let
// individual tests script per-endpoint response bodies; the *Status
// fields override the default 200. Response shapes are the real ones
// confirmed against live Amazon 2026-07-12.
type v2ListsHandler struct {
	t *testing.T

	listsBody []byte
	itemsBody []byte

	addStatus int
	addBody   []byte

	putStatus int
	putFunc   func(body map[string]any) []byte // nil => echo-free 200 {}

	deleteStatus int

	// Captured request details for assertions.
	gotPath   string
	gotMethod string
	gotUA     string
	gotQuery  string
	gotBody   map[string]any
}

func (h *v2ListsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.gotPath = r.URL.Path
	h.gotMethod = r.Method
	h.gotUA = r.Header.Get("User-Agent")
	h.gotQuery = r.URL.RawQuery

	var body map[string]any
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		if len(b) > 0 {
			_ = json.Unmarshal(b, &body)
		}
	}
	h.gotBody = body

	path := r.URL.Path
	w.Header().Set("Content-Type", "application/json")

	switch {
	case strings.HasSuffix(path, "/v2/lists/fetch"):
		_, _ = w.Write(h.listsBody)
	case strings.HasSuffix(path, "/items/fetch"):
		_, _ = w.Write(h.itemsBody)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/items"):
		status := h.addStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		if h.addBody != nil {
			_, _ = w.Write(h.addBody)
		}
	case r.Method == http.MethodPut:
		status := h.putStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		if h.putFunc != nil {
			_, _ = w.Write(h.putFunc(body))
		} else {
			_, _ = w.Write([]byte(`{}`))
		}
	case r.Method == http.MethodDelete:
		status := h.deleteStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
	default:
		h.t.Fatalf("unexpected request: %s %s", r.Method, path)
	}
}

func newV2ListsHandler(t *testing.T) *v2ListsHandler {
	t.Helper()
	listsBody, err := os.ReadFile("testdata/lists/v2_lists.json")
	require.NoError(t, err)
	itemsBody, err := os.ReadFile("testdata/lists/v2_items.json")
	require.NoError(t, err)
	return &v2ListsHandler{t: t, listsBody: listsBody, itemsBody: itemsBody}
}

func TestListsV2FetchAndItems(t *testing.T) {
	h := newV2ListsHandler(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, lists, 2)

	// Default (SHOP) list sorts first.
	shop := lists[0]
	assert.Equal(t, ListTypeShop, shop.Type)
	assert.Equal(t, "list-shop-1", shop.ID)
	assert.Equal(t, "", shop.Name)
	require.Len(t, shop.Items, 2)

	// Active-first ordering: milk (ACTIVE) before bread (COMPLETE).
	assert.Equal(t, "milk", shop.Items[0].Value)
	assert.False(t, shop.Items[0].Completed)
	assert.Equal(t, 3, shop.Items[0].Version)
	assert.Equal(t, "bread", shop.Items[1].Value)
	assert.True(t, shop.Items[1].Completed)
	assert.Equal(t, 5, shop.Items[1].Version)

	for _, it := range shop.Items {
		assert.Same(t, shop, it.list, "item %q missing back-ref", it.ID)
	}

	custom := lists[1]
	assert.Equal(t, ListTypeCustom, custom.Type)
	assert.Equal(t, "house to-do", custom.Name)
	require.Len(t, custom.Items, 2)
}

func TestListsV2_UsesFetchPathsAndUserAgent(t *testing.T) {
	h := newV2ListsHandler(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	_, err := c.Lists.Get(context.Background())
	require.NoError(t, err)

	// The last request observed is an items/fetch call (lists come
	// first, then per-list items); assert its shape here, and assert
	// the lists/fetch shape via a fresh single-list-less capture below.
	assert.True(t, strings.Contains(h.gotPath, "/items/fetch"), "unexpected path: %s", h.gotPath)
	assert.Equal(t, http.MethodPost, h.gotMethod)
	assert.Equal(t, "limit=100", h.gotQuery)
	assert.Contains(t, h.gotUA, "AmazonWebView")
}

func TestListsV2_FetchListsPathAndBody(t *testing.T) {
	var gotPath, gotMethod, gotUA, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotUA = r.Header.Get("User-Agent")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"listInfoList":[]}`))
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	assert.Empty(t, lists)

	assert.True(t, strings.HasSuffix(gotPath, "/alexashoppinglists/api/v2/lists/fetch"),
		"unexpected path: %s", gotPath)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Contains(t, gotUA, "AmazonWebView")
	assert.Equal(t, "{}", gotBody)
}

func TestListsService_Get_WrapsTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Hijack and close abruptly so the client sees EOF.
		hj, ok := w.(http.Hijacker)
		require.True(t, ok)
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	_, err := c.Lists.Get(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotConnect)
}

func TestListsService_Get_WrapsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	_, err := c.Lists.Get(context.Background())
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	assert.Equal(t, http.MethodPost, apiErr.Method)
	assert.ErrorIs(t, err, ErrCannotRetrieveData)
}

func TestList_AddItem_PostsItemsEndpointAndAppendsItem(t *testing.T) {
	h := newV2ListsHandler(t)
	// Real add-response shape (confirmed against live Amazon 2026-07-12):
	// the created item is nested under itemInfoList, alongside failures.
	h.addBody = []byte(`{"failures":[],"itemInfoList":[{"itemId":"a336-new","itemName":"eggs","itemStatus":"ACTIVE","itemType":"KEYWORD","version":1}]}`)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shop := lists[0]
	beforeLen := len(shop.Items)

	added, err := shop.AddItem(context.Background(), "eggs")
	require.NoError(t, err)
	require.NotNil(t, added)

	assert.Equal(t, http.MethodPost, h.gotMethod)
	assert.True(t, strings.HasSuffix(h.gotPath, "/v2/lists/"+shop.ID+"/items"),
		"unexpected path: %s", h.gotPath)

	items, ok := h.gotBody["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	entry, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "eggs", entry["itemName"])
	assert.Equal(t, "KEYWORD", entry["itemType"])

	// The returned item carries the REAL server-assigned itemId +
	// version parsed from itemInfoList[0] — not an empty placeholder.
	assert.Equal(t, "a336-new", added.ID)
	assert.NotEmpty(t, added.ID)
	assert.Equal(t, "eggs", added.Value)
	assert.False(t, added.Completed)
	assert.Equal(t, 1, added.Version)
	assert.Same(t, shop, added.list)
	assert.Len(t, shop.Items, beforeLen+1)
	assert.Same(t, added, shop.Items[beforeLen])
}

func TestList_AddItem_ErrorsWhenResponseMissingItemInfoList(t *testing.T) {
	h := newV2ListsHandler(t)
	h.addBody = []byte(`{"failures":[]}`) // 2xx but no itemInfoList
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shop := lists[0]

	added, err := shop.AddItem(context.Background(), "paper towels")
	require.Error(t, err)
	assert.Nil(t, added, "must not return a zero-ID item write methods could misuse")
	assert.ErrorIs(t, err, ErrCannotRetrieveData)
}

func TestList_Complete_PutsItemStatusCompleteWithVersionQuery(t *testing.T) {
	h := newV2ListsHandler(t)
	// Real PUT-response shape (confirmed against live Amazon 2026-07-12):
	// the updated item is nested under singular "itemInfo" with a bumped
	// version.
	h.putFunc = func(body map[string]any) []byte {
		return []byte(`{"itemInfo":{"itemId":"item-1","itemName":"milk","itemStatus":"COMPLETE","version":4}}`)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shop := lists[0]
	item := shop.Items[0] // milk, ACTIVE, version 3

	err = shop.Complete(context.Background(), item)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPut, h.gotMethod)
	assert.True(t, strings.HasSuffix(h.gotPath, "/v2/lists/"+shop.ID+"/items/"+item.ID),
		"unexpected path: %s", h.gotPath)
	assert.Equal(t, "version=3", h.gotQuery)

	updates, ok := h.gotBody["itemAttributesToUpdate"].([]any)
	require.True(t, ok)
	require.Len(t, updates, 1)
	entry, ok := updates[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "itemStatus", entry["type"])
	assert.Equal(t, "COMPLETE", entry["value"])

	assert.True(t, item.Completed)
	// Version bumped from the itemInfo response (3 -> 4), so a
	// subsequent Delete sends the correct version.
	assert.Equal(t, 4, item.Version)
}

func TestList_Complete_NoOpWhenAlreadyCompleted(t *testing.T) {
	h := newV2ListsHandler(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shop := lists[0]
	already := shop.Items[1] // bread, COMPLETE

	h.gotMethod = "" // reset capture
	err = shop.Complete(context.Background(), already)
	require.NoError(t, err)
	assert.Empty(t, h.gotMethod, "Complete on an already-completed item should not hit the network")
}

func TestList_Complete_LeavesItemUntouchedOnError(t *testing.T) {
	h := newV2ListsHandler(t)
	h.putStatus = http.StatusConflict
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shop := lists[0]
	item := shop.Items[0]
	originalCompleted := item.Completed
	originalVersion := item.Version

	err = shop.Complete(context.Background(), item)
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, originalCompleted, item.Completed)
	assert.Equal(t, originalVersion, item.Version)
}

func TestList_Uncomplete_MirrorsComplete(t *testing.T) {
	h := newV2ListsHandler(t)
	h.putFunc = func(body map[string]any) []byte {
		return []byte(`{"itemInfo":{"itemId":"item-2","itemName":"bread","itemStatus":"ACTIVE","version":6}}`)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shop := lists[0]
	item := shop.Items[1] // bread, COMPLETE

	err = shop.Uncomplete(context.Background(), item)
	require.NoError(t, err)

	updates, ok := h.gotBody["itemAttributesToUpdate"].([]any)
	require.True(t, ok)
	entry := updates[0].(map[string]any)
	assert.Equal(t, "ACTIVE", entry["value"])
	assert.False(t, item.Completed)
	assert.Equal(t, 6, item.Version)
}

func TestList_Rename_PutsItemNameAndUpdatesValue(t *testing.T) {
	h := newV2ListsHandler(t)
	h.putFunc = func(body map[string]any) []byte {
		return []byte(`{"itemInfo":{"itemId":"item-1","itemName":"organic milk","itemStatus":"ACTIVE","version":10}}`)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shop := lists[0]
	item := shop.Items[0]

	err = shop.Rename(context.Background(), item, "organic milk")
	require.NoError(t, err)

	updates, ok := h.gotBody["itemAttributesToUpdate"].([]any)
	require.True(t, ok)
	entry := updates[0].(map[string]any)
	assert.Equal(t, "itemName", entry["type"])
	assert.Equal(t, "organic milk", entry["value"])

	assert.Equal(t, "organic milk", item.Value)
	assert.Equal(t, 10, item.Version)
}

func TestList_Delete_SendsDeleteWithVersionQuery(t *testing.T) {
	h := newV2ListsHandler(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shop := lists[0]
	item := shop.Items[0]
	beforeLen := len(shop.Items)

	err = shop.Delete(context.Background(), item)
	require.NoError(t, err)

	assert.Equal(t, http.MethodDelete, h.gotMethod)
	assert.True(t, strings.HasSuffix(h.gotPath, "/v2/lists/"+shop.ID+"/items/"+item.ID),
		"unexpected path: %s", h.gotPath)
	assert.Equal(t, "version=3", h.gotQuery)
	assert.Len(t, shop.Items, beforeLen-1)
	for _, it := range shop.Items {
		assert.NotEqual(t, item.ID, it.ID, "deleted item still present")
	}
}

func TestList_CompleteThenDelete_UsesBumpedVersion(t *testing.T) {
	// A caller holding the item across Complete -> Delete must send the
	// version bumped by the Complete response on the Delete, not the
	// original fetched version. Confirms fix for the itemInfo version
	// parse (live Amazon 2026-07-12).
	h := newV2ListsHandler(t)
	h.putFunc = func(body map[string]any) []byte {
		return []byte(`{"itemInfo":{"itemId":"item-1","itemName":"milk","itemStatus":"COMPLETE","version":4}}`)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shop := lists[0]
	item := shop.Items[0] // version 3

	require.NoError(t, shop.Complete(context.Background(), item))
	require.Equal(t, 4, item.Version)

	require.NoError(t, shop.Delete(context.Background(), item))
	assert.Equal(t, http.MethodDelete, h.gotMethod)
	assert.Equal(t, "version=4", h.gotQuery, "Delete must send the version bumped by Complete")
}

func TestList_Delete_ItemNotInParentReturnsSentinel(t *testing.T) {
	h := newV2ListsHandler(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shop := lists[0]
	custom := lists[1]
	wrongItem := custom.Items[0]

	err = shop.Delete(context.Background(), wrongItem)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrListItemNotFound)

	err = shop.Delete(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrListItemNotFound)
}
