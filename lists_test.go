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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListItem_ServerShape_ReturnsDefensiveCopy(t *testing.T) {
	original := map[string]any{
		"id":             "item-1",
		"value":          "milk",
		"encryptedValue": "ENCRYPTED-MILK",
		"version":        float64(3),
	}
	item := &ListItem{
		ID:          "item-1",
		Value:       "milk",
		serverShape: original,
	}

	copy1 := item.ServerShape()
	assert.Equal(t, "milk", copy1["value"])

	// Mutating the returned copy must not affect the item's internal state.
	copy1["value"] = "stolen"
	copy1["newKey"] = "injected"

	copy2 := item.ServerShape()
	assert.Equal(t, "milk", copy2["value"], "internal serverShape was mutated by caller")
	_, hasInjected := copy2["newKey"]
	assert.False(t, hasInjected, "internal serverShape gained a key from caller mutation")

	// The original map (still held internally) must also be unchanged.
	assert.Equal(t, "milk", original["value"])
}

func TestListItem_ServerShape_NilReturnsEmptyMap(t *testing.T) {
	item := &ListItem{ID: "item-x", Value: "x"}
	// serverShape intentionally left nil.

	got := item.ServerShape()
	require.NotNil(t, got, "ServerShape() must return a non-nil map even when serverShape is nil")
	assert.Empty(t, got)

	// Mutating the returned empty map must not allocate-and-cache a
	// shape on the item.
	got["sneak"] = "in"
	got2 := item.ServerShape()
	_, leaked := got2["sneak"]
	assert.False(t, leaked, "mutating the empty map leaked back into item state")
}

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

func TestListsService_Get_ParsesBothListTypes(t *testing.T) {
	body, err := os.ReadFile("testdata/lists/get_response.json")
	require.NoError(t, err)

	var gotPath, gotMethod, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)

	assert.True(t, strings.HasSuffix(gotPath, "/alexashoppinglists/api/getlistitems"),
		"unexpected path: %s", gotPath)
	assert.Equal(t, http.MethodGet, gotMethod)
	// iOS-app UA per spec (legacy endpoint is permissive but we match
	// the shape that's been observed working).
	assert.Contains(t, gotUA, "AmazonWebView")

	require.Len(t, lists, 2)

	// Default (shopping) list first; remaining lists sorted by (Type, Name).
	shopping := lists[0]
	assert.Equal(t, ListTypeShopping, shopping.Type)
	assert.True(t, shopping.Default)
	assert.Equal(t, "", shopping.Name)
	assert.Equal(t, 42, shopping.Version)
	require.Len(t, shopping.Items, 2)

	todo := lists[1]
	assert.Equal(t, ListTypeCustom, todo.Type)
	assert.False(t, todo.Default)
	assert.Equal(t, "house to-do", todo.Name)
	assert.Equal(t, 7, todo.Version)
	require.Len(t, todo.Items, 2)

	// Items carry the back-ref to their parent.
	for _, item := range shopping.Items {
		assert.Same(t, shopping, item.list, "item %q missing back-ref", item.ID)
		assert.NotNil(t, item.serverShape, "item %q missing serverShape", item.ID)
	}
}

func TestListsService_Get_SortsItemsActiveFirst(t *testing.T) {
	body, err := os.ReadFile("testdata/lists/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)

	// SHOPPING_LIST: milk (active, created 2026-05-27) and bread
	// (completed). Active first.
	shopping := lists[0]
	assert.Equal(t, "milk", shopping.Items[0].Value)
	assert.False(t, shopping.Items[0].Completed)
	assert.Equal(t, "bread", shopping.Items[1].Value)
	assert.True(t, shopping.Items[1].Completed)

	// "house to-do": fix the door (active) before buy paint (completed).
	todo := lists[1]
	assert.Equal(t, "fix the door", todo.Items[0].Value)
	assert.False(t, todo.Items[0].Completed)
	assert.Equal(t, "buy paint", todo.Items[1].Value)
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
	assert.Equal(t, http.MethodGet, apiErr.Method)
	assert.ErrorIs(t, err, ErrCannotRetrieveData)
}

func TestList_AddItem_PostsAndAppendsItem(t *testing.T) {
	getBody, err := os.ReadFile("testdata/lists/get_response.json")
	require.NoError(t, err)

	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		switch {
		case strings.HasSuffix(gotPath, "/getlistitems"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(getBody)
		case strings.Contains(gotPath, "/addlistitem/"):
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id": "item-new",
				"value": "eggs",
				"categoryValue": "Dairy",
				"completed": false,
				"version": 1,
				"customerId": "S-CUSTOMER",
				"encryptedValue": "ENCRYPTED-EGGS",
				"createdDateTime": "2026-05-28T12:00:00.000Z",
				"updatedDateTime": "2026-05-28T12:00:00.000Z",
				"listItemMetadata": {}
			}`))
		}
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shopping := lists[0]
	beforeLen := len(shopping.Items)

	added, err := shopping.AddItem(context.Background(), "eggs")
	require.NoError(t, err)
	require.NotNil(t, added)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.True(t, strings.Contains(gotPath, "/alexashoppinglists/api/addlistitem/"+shopping.ID),
		"unexpected path: %s", gotPath)
	assert.Contains(t, gotBody, `"value":"eggs"`)
	assert.Contains(t, gotBody, `"type":"TASK"`)

	assert.Equal(t, "item-new", added.ID)
	assert.Equal(t, "eggs", added.Value)
	assert.False(t, added.Completed)
	assert.Equal(t, 1, added.Version)
	assert.NotNil(t, added.serverShape)
	assert.Len(t, shopping.Items, beforeLen+1)
	assert.Same(t, added, shopping.Items[beforeLen])
}

func TestList_AddItem_BackRefWiredOnReturnedItem(t *testing.T) {
	getBody, err := os.ReadFile("testdata/lists/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getlistitems"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(getBody)
		case strings.Contains(r.URL.Path, "/addlistitem/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"item-x","value":"x","completed":false,"version":1}`))
		}
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shopping := lists[0]

	added, err := shopping.AddItem(context.Background(), "x")
	require.NoError(t, err)
	assert.Same(t, shopping, added.list)
}

func TestList_Complete_PutsFullEchoWithFlippedFlag(t *testing.T) {
	getBody, err := os.ReadFile("testdata/lists/get_response.json")
	require.NoError(t, err)

	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getlistitems"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(getBody)
		case strings.HasSuffix(r.URL.Path, "/updatelistitem"):
			gotMethod = r.Method
			gotPath = r.URL.Path
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			w.Header().Set("Content-Type", "application/json")
			gotBody["version"] = float64(4)
			gotBody["updatedDateTime"] = "2026-05-28T13:00:00.000Z"
			out, _ := json.Marshal(gotBody)
			_, _ = w.Write(out)
		}
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shopping := lists[0]
	item := shopping.Items[0]

	err = shopping.Complete(context.Background(), item)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.True(t, strings.HasSuffix(gotPath, "/alexashoppinglists/api/updatelistitem"))
	assert.Equal(t, true, gotBody["completed"])
	assert.Equal(t, "ENCRYPTED-MILK", gotBody["encryptedValue"])
	assert.Equal(t, "S-CUSTOMER", gotBody["customerId"])

	assert.True(t, item.Completed)
	assert.Equal(t, 4, item.Version)
	assert.Equal(t,
		time.Date(2026, 5, 28, 13, 0, 0, 0, time.UTC),
		item.UpdatedAt.UTC())
}

func TestList_Complete_NoOpWhenAlreadyCompleted(t *testing.T) {
	getBody, err := os.ReadFile("testdata/lists/get_response.json")
	require.NoError(t, err)

	var updateCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getlistitems"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(getBody)
		case strings.HasSuffix(r.URL.Path, "/updatelistitem"):
			updateCalls++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shopping := lists[0]
	already := shopping.Items[1]

	err = shopping.Complete(context.Background(), already)
	require.NoError(t, err)
	assert.Zero(t, updateCalls, "Complete on an already-completed item should not hit the network")
}

func TestList_Complete_LeavesItemUntouchedOnError(t *testing.T) {
	getBody, err := os.ReadFile("testdata/lists/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getlistitems"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(getBody)
		case strings.HasSuffix(r.URL.Path, "/updatelistitem"):
			w.WriteHeader(http.StatusConflict)
		}
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shopping := lists[0]
	item := shopping.Items[0]
	originalCompleted := item.Completed
	originalVersion := item.Version

	err = shopping.Complete(context.Background(), item)
	require.Error(t, err)
	assert.Equal(t, originalCompleted, item.Completed)
	assert.Equal(t, originalVersion, item.Version)
}

func TestList_Uncomplete_MirrorsComplete(t *testing.T) {
	getBody, err := os.ReadFile("testdata/lists/get_response.json")
	require.NoError(t, err)

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getlistitems"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(getBody)
		case strings.HasSuffix(r.URL.Path, "/updatelistitem"):
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			gotBody["version"] = float64(6)
			gotBody["updatedDateTime"] = "2026-05-28T14:00:00.000Z"
			out, _ := json.Marshal(gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(out)
		}
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shopping := lists[0]
	item := shopping.Items[1]

	err = shopping.Uncomplete(context.Background(), item)
	require.NoError(t, err)
	assert.Equal(t, false, gotBody["completed"])
	assert.False(t, item.Completed)
	assert.Equal(t, 6, item.Version)
}

func TestList_Delete_SendsDeleteWithEchoBody(t *testing.T) {
	getBody, err := os.ReadFile("testdata/lists/get_response.json")
	require.NoError(t, err)

	var gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getlistitems"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(getBody)
		case strings.HasSuffix(r.URL.Path, "/deletelistitem"):
			gotMethod = r.Method
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shopping := lists[0]
	item := shopping.Items[0]

	err = shopping.Delete(context.Background(), item)
	require.NoError(t, err)

	assert.Equal(t, http.MethodDelete, gotMethod)
	assert.Equal(t, "item-shopping-1", gotBody["id"])
	assert.Equal(t, "ENCRYPTED-MILK", gotBody["encryptedValue"])
	assert.Equal(t, "S-CUSTOMER", gotBody["customerId"])
}

func TestList_Delete_RemovesFromParentItemsOnSuccess(t *testing.T) {
	getBody, err := os.ReadFile("testdata/lists/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getlistitems"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(getBody)
		case strings.HasSuffix(r.URL.Path, "/deletelistitem"):
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shopping := lists[0]
	target := shopping.Items[0]
	beforeLen := len(shopping.Items)

	err = shopping.Delete(context.Background(), target)
	require.NoError(t, err)
	assert.Len(t, shopping.Items, beforeLen-1)
	for _, it := range shopping.Items {
		assert.NotEqual(t, target.ID, it.ID, "deleted item still present")
	}
}

func TestList_Delete_ItemNotInParentReturnsSentinel(t *testing.T) {
	getBody, err := os.ReadFile("testdata/lists/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(getBody)
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shopping := lists[0]
	todo := lists[1]
	wrongItem := todo.Items[0]

	err = shopping.Delete(context.Background(), wrongItem)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrListItemNotFound)

	err = shopping.Delete(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrListItemNotFound)
}

func TestList_Rename_UpdatesValueAndVersion(t *testing.T) {
	getBody, err := os.ReadFile("testdata/lists/get_response.json")
	require.NoError(t, err)

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getlistitems"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(getBody)
		case strings.HasSuffix(r.URL.Path, "/updatelistitem"):
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			gotBody["version"] = float64(10)
			gotBody["updatedDateTime"] = "2026-05-28T15:00:00.000Z"
			out, _ := json.Marshal(gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(out)
		}
	}))
	t.Cleanup(srv.Close)

	c := newListsTestClient(t, srv)
	lists, err := c.Lists.Get(context.Background())
	require.NoError(t, err)
	shopping := lists[0]
	item := shopping.Items[0]

	err = shopping.Rename(context.Background(), item, "organic milk")
	require.NoError(t, err)
	assert.Equal(t, "organic milk", gotBody["value"])
	assert.Equal(t, "ENCRYPTED-MILK", gotBody["encryptedValue"])
	assert.Equal(t, "organic milk", item.Value)
	assert.Equal(t, 10, item.Version)
}
