package http2stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// ErrStreamReauthRequired is returned by Conn.Open when the directives
// stream responds 401/403/407.
var ErrStreamReauthRequired = errors.New("http2stream: stream requires reauth")

// Conn is a single in-flight directives stream. It is single-use: one
// successful Open followed by one Parts iteration. Re-opening on
// disconnect is the caller's job (we want every reconnect to refresh
// the token + recheck capabilities).
//
// Conn fields are populated by the caller before Open. Once Open
// returns, treat the fields as read-only.
type Conn struct {
	Client      *http.Client
	URL         string
	AccessToken string

	resp        *http.Response
	reader      *Reader
	connectedCh chan struct{}

	partsCh chan []byte

	errMu sync.Mutex
	err   error
}

// Open issues the GET and validates the response. On success the
// Connected channel is closed and Parts is ready to iterate.
func (c *Conn) Open(ctx context.Context) error {
	c.connectedCh = make(chan struct{})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return fmt.Errorf("http2stream: build stream request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("Accept", "multipart/related")
	req.Header.Set("Accept-Encoding", "identity") // no gzip — we stream

	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("http2stream: open stream: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusProxyAuthRequired:
		_ = resp.Body.Close()
		return fmt.Errorf("%w: status %d", ErrStreamReauthRequired, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return fmt.Errorf("http2stream: open stream: unexpected status %d", resp.StatusCode)
	}

	r, err := NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if err != nil {
		_ = resp.Body.Close()
		return err
	}
	c.resp = resp
	c.reader = r
	close(c.connectedCh)

	c.partsCh = make(chan []byte)
	go c.pump(ctx)
	return nil
}

// Connected returns a channel that closes once Open has successfully
// validated the HTTP response and is ready to deliver parts. Used by
// the ping loop to gate its first ping.
func (c *Conn) Connected() <-chan struct{} { return c.connectedCh }

// Parts returns the channel of decoded JSON bodies. The channel
// closes when the upstream stream ends (clean EOF, error, or ctx
// cancellation). After close, inspect Err for a non-nil terminal
// error.
func (c *Conn) Parts() <-chan []byte { return c.partsCh }

// Err returns the terminal error of the stream, if any. Returns nil on
// clean EOF. Safe to call concurrently after the parts channel has
// closed.
func (c *Conn) Err() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.err
}

// Close releases the underlying body. Idempotent.
func (c *Conn) Close() error {
	if c.resp != nil {
		return c.resp.Body.Close()
	}
	return nil
}

func (c *Conn) pump(ctx context.Context) {
	defer close(c.partsCh)
	for {
		select {
		case <-ctx.Done():
			c.setErr(ctx.Err())
			return
		default:
		}
		body, err := c.reader.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			c.setErr(err)
			return
		}
		select {
		case c.partsCh <- body:
		case <-ctx.Done():
			c.setErr(ctx.Err())
			return
		}
	}
}

func (c *Conn) setErr(err error) {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	if c.err == nil {
		c.err = err
	}
}
