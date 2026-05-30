package alexactl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/http2stream"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// Reconnect tunables per spec §F.
const (
	reconnectMin = 5 * time.Second
	reconnectMax = 600 * time.Second
	pingInterval = 280 * time.Second
	openTimeout  = 30 * time.Second //nolint:unused // tunable per spec §F
)

// defaultSleep is the production sleepFn. Honours ctx cancellation
// and returns ctx.Err() when cancelled.
func defaultSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// PushService streams real-time push events from Amazon's AVS
// directives endpoint. Construct only via NewClient; safe for
// concurrent use across multiple Subscribe callers.
type PushService struct {
	wrapper *httpwrapper.Wrapper

	sessMu sync.RWMutex
	sess   *Session

	// HTTP client used for the directives stream and the /ping POST.
	// Separate from wrapper's client because we don't want the
	// transparent 401 refresh middleware on the push surface — push
	// owns its own reauth flow.
	streamClient *http.Client

	// capabilitiesRegistered is the in-process cache. Initialised
	// from session.CapabilitiesRegistered on every setSession. Only
	// transitions false → true.
	capabilitiesRegistered atomic.Bool

	// Lifecycle state. streamGoroutineActive is set to 1 while a
	// streamLoop is in flight (used by Subscribe to avoid starting
	// multiple loops).
	lifecycleMu           sync.Mutex
	streamCancel          context.CancelFunc
	streamGoroutineActive atomic.Bool

	// Per-subscriber bookkeeping. Each subscriber has a buffered
	// channel + a ctx-watcher goroutine. Subscribers are appended
	// while the lifecycleMu is held; the streamLoop fans events out
	// under a read lock.
	subsMu sync.RWMutex
	subs   []*subscriber

	// Test seams. nowFn is reserved for future use (spec §J lists
	// it as a tunable) but not wired in this release; add it when a
	// caller actually needs deterministic timestamps.
	sleepFn       func(context.Context, time.Duration) error
	initialSyncFn func(context.Context) (map[string]*VolumeState, error)

	// capabilitiesURLFn overrides the capabilities PUT URL. Set by
	// tests; production code uses the default builder.
	capabilitiesURLFn func() string

	// streamURLFn overrides the directives stream URL. Set by tests.
	streamURLFn func() string

	// pingURLFn overrides the /ping POST URL. Set by tests.
	pingURLFn func() string
}

// subscriber represents one live Subscribe caller.
type subscriber struct {
	ch                   chan PushEvent
	cfg                  *subscribeConfig
	initialSyncTriggered atomic.Bool
}

func (s *PushService) setSession(sess *Session) {
	s.sessMu.Lock()
	s.sess = sess
	s.sessMu.Unlock()
	if sess != nil && sess.CapabilitiesRegistered {
		s.capabilitiesRegistered.Store(true)
	}
}

func (s *PushService) session() *Session {
	s.sessMu.RLock()
	defer s.sessMu.RUnlock()
	return s.sess
}

// registerCapabilities PUTs the static capabilities payload to
// /v1/devices/@self/capabilities. On 204 it sets the in-process flag
// AND mutates session.CapabilitiesRegistered so callers serialising
// the session next will persist the bit.
//
// Returns nil if the in-process cache already says registered (cheap
// fast path).
func (s *PushService) registerCapabilities(ctx context.Context, accessToken string) error {
	if s.capabilitiesRegistered.Load() {
		return nil
	}

	body, err := json.Marshal(amzconst.DeviceCapabilities())
	if err != nil {
		return fmt.Errorf("alexactl: marshal capabilities: %w", err)
	}

	url := s.capabilitiesURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("alexactl: build capabilities request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return &APIError{
			Category:   ErrCannotRegisterDevice,
			Method:     http.MethodPut,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}

	s.capabilitiesRegistered.Store(true)
	// Mutate the in-memory session so the caller's next persist
	// captures the bit. Single-writer (this method is the only place
	// that writes the field), readers tolerate stale-false reads.
	if sess := s.session(); sess != nil {
		sess.CapabilitiesRegistered = true
	}
	return nil
}

// capabilitiesURL builds the absolute URL for the capabilities PUT.
// Overridable through the test seam by setting capabilitiesURLFn.
func (s *PushService) capabilitiesURL() string {
	if s.capabilitiesURLFn != nil {
		return s.capabilitiesURLFn()
	}
	return "https://api.amazonalexa.com" + amzconst.URICapabilities
}

// ===== Subscribe + lifecycle =====

// Subscribe registers a new subscriber and returns its dedicated
// receive channel. Each call returns a NEW channel; one background
// goroutine maintains a single upstream stream and fans every parsed
// event out to all live subscribers.
//
// The first Subscribe starts the stream loop. The last unsubscribe
// (all subscriber ctxs cancelled) stops it. Reauth-required closes
// all subscriber channels after delivering a *ReauthRequiredEvent.
func (s *PushService) Subscribe(ctx context.Context, opts ...SubscribeOption) (<-chan PushEvent, error) {
	cfg := defaultSubscribeConfig()
	for _, o := range opts {
		o.apply(cfg)
	}

	sess := s.session()
	if sess == nil {
		return nil, fmt.Errorf("alexactl: Push.Subscribe before Auth.Login/Restore: %w", ErrWrongMethod)
	}

	sub := &subscriber{ch: make(chan PushEvent, cfg.chanCapacity), cfg: cfg}

	s.subsMu.Lock()
	s.subs = append(s.subs, sub)
	s.subsMu.Unlock()

	// Watcher: unregister the subscriber on ctx cancel.
	go s.watchSubscriber(ctx, sub)

	s.startStreamLoopOnce() //nolint:contextcheck // stream loop uses context.Background() by design — independent of caller ctx
	return sub.ch, nil
}

func (s *PushService) watchSubscriber(ctx context.Context, sub *subscriber) {
	<-ctx.Done()
	s.removeSubscriber(sub)
}

func (s *PushService) removeSubscriber(sub *subscriber) {
	s.subsMu.Lock()
	for i, x := range s.subs {
		if x == sub {
			s.subs = append(s.subs[:i], s.subs[i+1:]...)
			break
		}
	}
	left := len(s.subs)
	s.subsMu.Unlock()
	// Channel may already be closed by notifyReauthAndClose; safe to
	// ignore the panic.
	func() {
		defer func() { _ = recover() }()
		close(sub.ch)
	}()
	if left == 0 {
		s.stopStreamLoop()
	}
}

func (s *PushService) startStreamLoopOnce() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.streamGoroutineActive.Load() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.streamCancel = cancel
	s.streamGoroutineActive.Store(true)
	go func() {
		defer s.streamGoroutineActive.Store(false)
		s.streamLoop(ctx)
	}()
}

func (s *PushService) stopStreamLoop() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.streamCancel != nil {
		s.streamCancel()
		s.streamCancel = nil
	}
}

// streamLoop is the lifetime of the upstream connection. Task 8
// adds reauth handling; Task 11 adds ping integration. For this task
// the loop reconnects with exponential backoff until ctx cancels.
func (s *PushService) streamLoop(ctx context.Context) {
	logger := s.logger(ctx)
	sleep := s.sleepFn
	if sleep == nil {
		sleep = defaultSleep
	}

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		sess := s.session()
		if sess == nil {
			logger.WarnContext(ctx, "alexactl: push streamLoop with no session — exiting")
			return
		}
		accessToken := sess.AccessToken

		if err := s.registerCapabilities(ctx, accessToken); err != nil {
			logger.WarnContext(ctx,
				"alexactl: capabilities registration failed",
				"err", err)
			if !s.backoffSleep(ctx, sleep, attempt) {
				return
			}
			attempt++
			continue
		}

		conn := &http2stream.Conn{
			Client:      s.streamClient,
			URL:         s.streamURL(),
			AccessToken: accessToken,
		}
		err := conn.Open(ctx)
		if errors.Is(err, http2stream.ErrStreamReauthRequired) {
			logger.WarnContext(ctx,
				"alexactl: push stream requires reauth; notifying subscribers",
				"err", err)
			s.notifyReauthAndClose(ctx)
			_ = conn.Close()
			return
		}
		if err != nil {
			logger.WarnContext(ctx, "alexactl: stream open failed",
				"err", err)
			_ = conn.Close()
			if !s.backoffSleep(ctx, sleep, attempt) {
				return
			}
			attempt++
			continue
		}

		// Successful open — provisionally clear attempt; runConnection
		// returns the post-cycle value (0 if a ping succeeded; previous
		// otherwise).
		attempt = s.runConnection(ctx, conn, attempt)
		_ = conn.Close()

		// Stream ended; loop to reconnect (unless ctx cancelled).
	}
}

// backoffSleep waits min(reconnectMin*2^attempt, reconnectMax). Returns
// false if the sleep was cancelled by ctx (caller should exit).
func (s *PushService) backoffSleep(ctx context.Context, sleep func(context.Context, time.Duration) error, attempt int) bool {
	delay := reconnectMin << attempt
	if delay <= 0 || delay > reconnectMax { // <=0 guards overflow
		delay = reconnectMax
	}
	return sleep(ctx, delay) == nil
}

// runConnection pumps parts from an already-open Conn until it
// terminates (clean EOF, error, or ctx cancel). It also runs a
// keepalive ping goroutine gated on conn.Connected().
//
// Returns the new attempt counter — 0 on healthy ping (resets
// backoff) or the previous attempt count on a no-ping run.
func (s *PushService) runConnection(ctx context.Context, conn *http2stream.Conn, prevAttempt int) int {
	logger := s.logger(ctx)
	pingCtx, cancelPing := context.WithCancel(ctx)
	var pingHealthy atomic.Bool
	go s.pingLoop(pingCtx, conn, &pingHealthy)
	defer cancelPing()

	for part := range conn.Parts() {
		logPushPart(ctx, logger, part)
		events, err := DecodeRenderingUpdate(part)
		if err != nil {
			logger.WarnContext(ctx, "alexactl: decode part failed",
				"err", err)
			continue
		}
		for _, evt := range events {
			s.broadcast(ctx, evt)
		}
	}
	if err := conn.Err(); err != nil {
		logger.DebugContext(ctx, "alexactl: stream ended with error",
			"err", err)
	}
	if pingHealthy.Load() {
		return 0
	}
	return prevAttempt
}

// pingLoop sends a /ping POST every pingInterval after Connected
// fires, until pingCtx is cancelled or the conn closes its parts
// channel. Healthy pings flip pingHealthy true so runConnection
// can reset the reconnect counter on stream end.
func (s *PushService) pingLoop(ctx context.Context, conn *http2stream.Conn, pingHealthy *atomic.Bool) {
	logger := s.logger(ctx)
	sleep := s.sleepFn
	if sleep == nil {
		sleep = defaultSleep
	}

	// Wait for the stream to be open.
	select {
	case <-conn.Connected():
	case <-ctx.Done():
		return
	}

	for {
		if err := sleep(ctx, pingInterval); err != nil {
			return
		}
		sess := s.session()
		if sess == nil {
			return
		}
		err := http2stream.Ping(ctx, s.streamClient, s.pingURL(), sess.AccessToken)
		if err != nil {
			if errors.Is(err, http2stream.ErrPingReauthRequired) {
				logger.WarnContext(ctx,
					"alexactl: ping requires reauth — closing stream so streamLoop reauth branch fires",
					"err", err)
				_ = conn.Close()
				return
			}
			logger.WarnContext(ctx, "alexactl: ping error (will continue)",
				"err", err)
			continue
		}
		pingHealthy.Store(true)
		logger.DebugContext(ctx, "alexactl: push ping ok")
	}
}

// logPushPart logs a single multipart part from the AVS directives
// stream at DEBUG, structured as map[string]any so the scrub.Handler
// slog middleware can redact any sensitive fields. No-op when DEBUG is
// disabled. Non-JSON parts (shouldn't happen — the parser enforces
// application/json) log size only.
//
// This is the push-stream analogue of httpwrapper.LogResponseBody: a
// single point where stream traffic becomes structured DEBUG output,
// safe for production logs.
func logPushPart(ctx context.Context, logger *slog.Logger, part []byte) {
	if !logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	var body map[string]any
	if err := json.Unmarshal(part, &body); err != nil {
		logger.DebugContext(ctx, "alexactl: push body (non-json)",
			"bytes", len(part))
		return
	}
	logger.DebugContext(ctx, "alexactl: push body",
		"body", body)
}

// pingURL is the AVS site /ping endpoint. Test seam parallel to
// streamURLFn / capabilitiesURLFn.
func (s *PushService) pingURL() string {
	if s.pingURLFn != nil {
		return s.pingURLFn()
	}
	sess := s.session()
	site := strings.Replace(amzconst.HTTP2SiteTemplate, "{region}",
		strings.ToLower(sess.CustomerInfo.HomeRegion), 1)
	return site + "/ping"
}

func (s *PushService) broadcast(ctx context.Context, evt PushEvent) {
	s.subsMu.RLock()
	subs := make([]*subscriber, len(s.subs))
	copy(subs, s.subs)
	s.subsMu.RUnlock()

	for _, sub := range subs {
		s.deliverWithDropOldest(ctx, sub, evt)
		if sub.cfg.initialSyncOnFirstEvent &&
			sub.initialSyncTriggered.CompareAndSwap(false, true) {
			go s.runInitialSync(ctx, sub)
		}
	}
}

// deliverWithDropOldest tries a non-blocking send. If the buffer is
// full it discards the oldest queued event and retries. Worst case:
// drop both the new event and one stale one if the consumer races —
// acceptable because dropping is the documented backpressure mode.
func (s *PushService) deliverWithDropOldest(ctx context.Context, sub *subscriber, evt PushEvent) {
	logger := s.logger(ctx)
	select {
	case sub.ch <- evt:
		return
	default:
	}
	// Channel full — drop the oldest (one receive) and retry once.
	select {
	case <-sub.ch:
		logger.WarnContext(ctx,
			"alexactl: push subscriber backpressure — dropped oldest event")
	default:
		// Channel drained between checks; fine.
	}
	select {
	case sub.ch <- evt:
	default:
		logger.WarnContext(ctx,
			"alexactl: push subscriber backpressure — dropped new event")
	}
}

func (s *PushService) logger(ctx context.Context) *slog.Logger {
	if s.wrapper != nil {
		return s.wrapper.Logger(ctx)
	}
	return slog.Default()
}

// streamURL returns the directives endpoint URL. Test seam analogous
// to capabilitiesURLFn.
func (s *PushService) streamURL() string {
	if s.streamURLFn != nil {
		return s.streamURLFn()
	}
	sess := s.session()
	site := strings.Replace(amzconst.HTTP2SiteTemplate, "{region}",
		strings.ToLower(sess.CustomerInfo.HomeRegion), 1)
	return site + "/v" + amzconst.HTTP2DirectivesVersion + "/directives"
}

// notifyReauthAndClose delivers a *ReauthRequiredEvent to every live
// subscriber (best-effort drop-oldest), then closes every subscriber
// channel. After this call the subscriber slice is empty.
func (s *PushService) notifyReauthAndClose(ctx context.Context) {
	evt := &ReauthRequiredEvent{}
	s.broadcast(ctx, evt)
	s.closeAllSubscribers()
}

// runInitialSync pulls current per-device volume state and synthesises
// one *VolumeChangeEvent per device, delivered only to this subscriber.
// Failures are logged and swallowed — initial sync is best-effort.
func (s *PushService) runInitialSync(ctx context.Context, sub *subscriber) {
	logger := s.logger(ctx)
	syncFn := s.initialSyncFn
	if syncFn == nil {
		// Default wiring is set by Client at construction time. If
		// it is still nil here, we've nothing to do.
		logger.WarnContext(ctx,
			"alexactl: initial sync requested but no initialSyncFn wired")
		return
	}
	states, err := syncFn(ctx)
	if err != nil {
		logger.WarnContext(ctx, "alexactl: initial sync failed",
			"err", err)
		return
	}
	for serial, st := range states {
		evt := NewVolumeChangeEvent(
			NewPushEventBase(serial, map[string]any{
				"volumeSetting": float64(st.Volume),
				"isMuted":       st.Muted,
			}, PushVolumeChange),
			st.Volume, st.Muted,
		)
		s.deliverWithDropOldest(ctx, sub, evt)
	}
}

func (s *PushService) closeAllSubscribers() {
	s.subsMu.Lock()
	subs := s.subs
	s.subs = nil
	s.subsMu.Unlock()
	for _, sub := range subs {
		close(sub.ch)
	}
}
