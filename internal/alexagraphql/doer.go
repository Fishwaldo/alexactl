package alexagraphql

import (
	"fmt"
	"io"
	"net/http"

	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// Doer is the genqlient transport interface. It is satisfied by
// *http.Client; we implement it on top of *httpwrapper.Wrapper so
// generated-client calls inherit auth refresh, retry, scrub, and
// body logging from the rest of the library.
type Doer struct {
	wrapper *httpwrapper.Wrapper
}

// NewDoer returns a Doer that forwards every request through w. The
// returned value satisfies the genqlient `graphql.Doer` interface.
func NewDoer(w *httpwrapper.Wrapper) *Doer {
	return &Doer{wrapper: w}
}

// Do translates an *http.Request into an httpwrapper.Request and forwards
// it. The request body bytes pass through verbatim via RawBody — genqlient
// has already marshaled the GraphQL payload, so we must not re-encode.
func (d *Doer) Do(req *http.Request) (*http.Response, error) {
	var rawBody []byte
	if req.Body != nil {
		var err error
		rawBody, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("alexagraphql: read request body: %w", err)
		}
		_ = req.Body.Close()
	}

	headers := http.Header{}
	for k, vs := range req.Header {
		headers[k] = append(headers[k], vs...)
	}

	wrappedReq := httpwrapper.Request{
		Method:       req.Method,
		URL:          req.URL.String(),
		ExtraHeaders: headers,
	}
	if len(rawBody) > 0 {
		wrappedReq.RawBody = rawBody
	}

	return d.wrapper.Do(req.Context(), wrappedReq)
}
