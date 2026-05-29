package amazondevices

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/cache"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// marshalRoutinesEntries / unmarshalRoutinesEntries persist the
// sequence.routines cache via the engine's Persistence adapter.
func marshalRoutinesEntries(in map[string]cache.Entry[map[string]map[string]any]) ([]byte, error) {
	return json.Marshal(in)
}

func unmarshalRoutinesEntries(b []byte) (map[string]cache.Entry[map[string]map[string]any], error) {
	out := map[string]cache.Entry[map[string]map[string]any]{}
	err := json.Unmarshal(b, &out)
	return out, err
}

// SequenceService dispatches Alexa sequence operations (Speak,
// Announcement, Sound, Music, TextCommand, LaunchSkill, Volume, Stop,
// Routines) to one or more devices via /api/behaviors/preview.
//
// Operations issued via Send are buffered for up to SequenceBatchDelay
// (300ms) and posted as a single sequence. Identical operations on
// distinct devices are wrapped in a ParallelNode so Amazon executes
// them concurrently. Flush preempts the timer and drains synchronously.
//
// Routines() lazy-loads and caches the enabled-routine set so that
// CallRoutine() can look up the startNode payload.
//
// Safe for concurrent use.
type SequenceService struct {
	wrapper *httpwrapper.Wrapper

	mu      sync.Mutex
	session *Session

	// buffer of pending operations, drained by processBatch or Flush.
	buffer []sequenceOp

	// batchPending is true while a processBatch goroutine is sleeping.
	// New Send calls during this window accumulate into buffer.
	batchPending bool

	// batchGen is incremented by Flush to invalidate any pending
	// processBatch goroutine that hasn't woken yet.
	batchGen uint64

	// routinesCache: name -> sequence object (the full sequence map,
	// indexed for "startNode" lookup by buildOperationNode). Engine-managed
	// (single sentinel key "").
	routinesCache *cache.Cache[map[string]map[string]any]

	// Test seam — overridden by tests for deterministic timing.
	// Production leaves nil; processBatch uses time.NewTimer.
	sleepFn func(ctx context.Context, d time.Duration) error
}

// sequenceOp is one buffered operation waiting to be POSTed.
type sequenceOp struct {
	messageType   string
	messageBody   any
	providerID    string
	serial        string
	deviceType    string
	operationNode map[string]any
}

// setSession swaps in a fresh *Session pointer. Invoked by AuthService
// after Login / Restore / refreshAccessToken via the broadcast pipeline.
func (s *SequenceService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
}

// snapshotSession returns the current *Session pointer under the lock.
// Returned pointer is treated as immutable (Auth swaps pointers, never
// mutates in place).
func (s *SequenceService) snapshotSession() *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session
}

// sleep waits d, honoring the test seam if set.
func (s *SequenceService) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	fn := s.sleepFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// buildOperationNode constructs the OpaquePayloadOperationNode envelope
// for one (device, messageType, messageBody, providerID) tuple. Port of
// the Python sequence.py _build_operation_node — all branches except
// Routines (which lands in Task 10).
func (s *SequenceService) buildOperationNode(ctx context.Context, d *Device, messageType string, messageBody any, providerID string) (map[string]any, error) {
	sess := s.snapshotSession()
	if sess == nil || sess.AccountCustomerID == "" {
		return nil, fmt.Errorf("%w: missing customer ID", ErrCannotAuthenticate)
	}

	base := map[string]any{
		"deviceType":         d.DeviceType,
		"deviceSerialNumber": d.SerialNumber,
		"locale":             s.wrapper.Language(),
		"customerId":         sess.AccountCustomerID,
	}

	var payload map[string]any
	switch messageType {
	case string(SeqSpeak):
		payload = mergeMap(base, map[string]any{
			"textToSpeak": messageBody,
			"target": map[string]any{
				"customerId": sess.AccountCustomerID,
				"devices": []any{
					map[string]any{
						"deviceSerialNumber": d.SerialNumber,
						"deviceTypeId":       d.DeviceType,
					},
				},
			},
			"skillId": "amzn1.ask.1p.saysomething",
		})
	case string(SeqAnnouncement):
		members := sortedKeys(d.ClusterMembers)
		if len(members) == 0 {
			members = []string{d.SerialNumber}
		}
		devices := make([]any, 0, len(members))
		for _, serial := range members {
			dt := d.ClusterMembers[serial]
			if dt == "" {
				dt = d.DeviceType
			}
			devices = append(devices, map[string]any{
				"deviceSerialNumber": serial,
				"deviceTypeId":       dt,
			})
		}
		payload = mergeMap(base, map[string]any{
			"expireAfter": "PT5S",
			"content": []any{
				map[string]any{
					"locale":  s.wrapper.Language(),
					"display": map[string]any{"title": "goamazondevices", "body": messageBody},
					"speak":   map[string]any{"type": "text", "value": messageBody},
				},
			},
			"target": map[string]any{
				"customerId": sess.AccountCustomerID,
				"devices":    devices,
			},
			"skillId": "amzn1.ask.1p.routines.messaging",
		})
	case string(SeqSound):
		payload = mergeMap(base, map[string]any{
			"soundStringId": messageBody,
			"skillId":       "amzn1.ask.1p.sound",
		})
	case string(SeqMusic):
		payload = mergeMap(base, map[string]any{
			"searchPhrase":          messageBody,
			"sanitizedSearchPhrase": messageBody,
			"musicProviderId":       providerID,
		})
	case string(SeqTextCommand):
		payload = mergeMap(base, map[string]any{
			"skillId": "amzn1.ask.1p.tellalexa",
			"text":    messageBody,
		})
	case string(SeqLaunchSkill):
		var skillURI string
		switch b := messageBody.(type) {
		case string:
			skillURI = "connection://AMAZON.Launch/" + b
		default:
			skillURI = fmt.Sprintf("connection://AMAZON.Launch/%v", b)
		}
		payload = mergeMap(base, map[string]any{
			"targetDevice": map[string]any{
				"deviceType":         d.DeviceType,
				"deviceSerialNumber": d.SerialNumber,
			},
			"connectionRequest": map[string]any{"uri": skillURI},
		})
	case string(SeqVolume):
		payload = mergeMap(base, map[string]any{"value": messageBody})
	case string(SeqStop):
		payload = mergeMap(base, map[string]any{
			"devices": []any{
				map[string]any{
					"deviceSerialNumber": d.SerialNumber,
					"deviceType":         d.DeviceType,
				},
			},
			"skillId": "amzn1.ask.1p.alexadevicecontrols",
		})
	case string(SeqRoutine):
		name, ok := messageBody.(string)
		if !ok {
			return nil, fmt.Errorf("amazondevices: sequence: routine name must be a string")
		}
		cached, _, gerr := s.routinesCache.Get(ctx, "")
		if gerr != nil {
			return nil, gerr
		}
		seq, found := cached[name]
		if !found {
			return nil, fmt.Errorf("amazondevices: sequence: unknown routine %q (run Routines first)", name)
		}
		startNode, _ := seq["startNode"].(map[string]any)
		if startNode == nil {
			return nil, fmt.Errorf("amazondevices: sequence: routine %q has no startNode", name)
		}
		return replaceRoutinePlaceholders(deepCopyMap(startNode), d), nil
	default:
		if amzconst.IsAlexaInfoSkill(messageType) {
			payload = base
			break
		}
		return nil, fmt.Errorf("amazondevices: sequence: unrecognised message type %q", messageType)
	}

	return map[string]any{
		"@type":            amzconst.OperationNodeAttr,
		"type":             messageType,
		"operationPayload": payload,
	}, nil
}

// mergeMap returns a new map with both inputs' entries; rhs wins on key
// collisions. Inputs are not mutated.
func mergeMap(lhs, rhs map[string]any) map[string]any {
	out := make(map[string]any, len(lhs)+len(rhs))
	for k, v := range lhs {
		out[k] = v
	}
	for k, v := range rhs {
		out[k] = v
	}
	return out
}

// sortedKeys returns the keys of m in lexical order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// postSequences POSTs the operation nodes wrapped in a SerialNode
// envelope to /api/behaviors/preview. The optimisation pass (parallel
// wrapping of identical ops across devices) lives in Task 8 —
// postSequences itself just serialises whatever nodes it is handed.
func (s *SequenceService) postSequences(ctx context.Context, ops []sequenceOp) error {
	if len(ops) == 0 {
		return nil
	}

	nodes := s.optimiseSequenceNodes(ops)

	seq := map[string]any{
		"@type": amzconst.SequenceTypeAttr,
		"startNode": map[string]any{
			"@type":          amzconst.SerialNodeAttr,
			"nodesToExecute": nodes,
		},
	}
	seqJSON, err := json.Marshal(seq)
	if err != nil {
		return fmt.Errorf("amazondevices: sequence marshal: %w", err)
	}

	body := map[string]any{
		"behaviorId":   "PREVIEW",
		"sequenceJson": string(seqJSON),
		"status":       "ENABLED",
	}

	s.wrapper.Logger(ctx).DebugContext(ctx, "amazondevices: sending sequence",
		"op_count", len(ops))

	url := fmt.Sprintf("https://alexa.amazon.%s%s", s.wrapper.Domain(), amzconst.URIBehaviorsPreview)
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:   http.MethodPost,
		URL:      url,
		JSONBody: body,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodPost,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	return nil
}

// Send enqueues an operation against a device for the next batch. The
// HTTP POST happens after sequenceBatchDelay (300ms) unless Flush is
// called sooner. Returns the build error synchronously (unknown
// messageType, missing session) but does NOT report the HTTP outcome —
// async failures are logged at WARN level. Callers wanting sync
// semantics call Flush after Send.
func (s *SequenceService) Send(ctx context.Context, d *Device, kind SequenceType, body any, providerID string) error {
	node, err := s.buildOperationNode(ctx, d, string(kind), body, providerID)
	if err != nil {
		return err
	}
	op := sequenceOp{
		messageType:   string(kind),
		messageBody:   body,
		providerID:    providerID,
		serial:        d.SerialNumber,
		deviceType:    d.DeviceType,
		operationNode: node,
	}

	s.mu.Lock()
	s.buffer = append(s.buffer, op)
	if s.batchPending {
		s.mu.Unlock()
		return nil
	}
	s.batchPending = true
	gen := s.batchGen
	s.mu.Unlock()

	// processBatch outlives the caller's ctx by design — the deferred
	// POST runs in background with a context derived from the wrapper's
	// own. Callers wanting cancellation use Flush(ctx).
	go s.processBatch(context.Background(), gen)
	return nil
}

// processBatch sleeps the debounce window, then drains the buffer and
// posts (unless Flush already won the race, signaled by a bumped
// batchGen).
func (s *SequenceService) processBatch(ctx context.Context, gen uint64) {
	_ = s.sleep(ctx, amzconst.SequenceBatchDelay)

	s.mu.Lock()
	if s.batchGen != gen {
		// Flush won the race — nothing to do.
		s.mu.Unlock()
		return
	}
	ops := s.buffer
	s.buffer = nil
	s.batchPending = false
	s.mu.Unlock()

	if len(ops) == 0 {
		return
	}
	if err := s.postSequences(ctx, ops); err != nil {
		s.wrapper.Logger(ctx).WarnContext(ctx,
			"amazondevices: sequence batch failed",
			"err", err, "op_count", len(ops))
	}
}

// Flush forces any pending batch to be sent synchronously. Used by
// MediaService.Speak/Announce/SetVolume facades to preserve their
// Phase-1 sync semantics, and by callers wanting deterministic
// error-propagation. Idempotent on an empty buffer.
//
// Flush bumps batchGen so any sleeping processBatch goroutine will
// notice its generation is stale and exit cleanly without re-draining
// the (now-empty) buffer.
func (s *SequenceService) Flush(ctx context.Context) error {
	s.mu.Lock()
	if len(s.buffer) == 0 && !s.batchPending {
		s.mu.Unlock()
		return nil
	}
	ops := s.buffer
	s.buffer = nil
	s.batchPending = false
	s.batchGen++
	s.mu.Unlock()

	if len(ops) == 0 {
		return nil
	}
	return s.postSequences(ctx, ops)
}

// optimiseSequenceNodes groups consecutive ops by (messageType,
// messageBody, providerID); when a group hits >=2 distinct devices it
// is wrapped in a ParallelNode so Amazon executes them concurrently.
// Otherwise each op's operationNode is emitted inline, in original
// order. Port of upstream sequence.py _optimise_sequence_nodes.
func (s *SequenceService) optimiseSequenceNodes(ops []sequenceOp) []any {
	out := make([]any, 0, len(ops))
	i := 0
	for i < len(ops) {
		j := i + 1
		for j < len(ops) && sameOpKey(ops[i], ops[j]) {
			j++
		}
		group := ops[i:j]
		serials := map[string]struct{}{}
		for _, op := range group {
			serials[op.serial] = struct{}{}
		}
		if len(group) > 1 && len(serials) == len(group) {
			children := make([]any, 0, len(group))
			for _, op := range group {
				children = append(children, op.operationNode)
			}
			out = append(out, map[string]any{
				"@type":          amzconst.ParallelNodeAttr,
				"nodesToExecute": children,
			})
		} else {
			for _, op := range group {
				out = append(out, op.operationNode)
			}
		}
		i = j
	}
	return out
}

func sameOpKey(a, b sequenceOp) bool {
	return a.messageType == b.messageType &&
		a.providerID == b.providerID &&
		fmt.Sprintf("%v", a.messageBody) == fmt.Sprintf("%v", b.messageBody)
}

// Routines returns the names of ENABLED routines on the account.
// First call fetches; subsequent calls within TTL return the cached
// names. Use RefreshRoutines to force a re-fetch.
func (s *SequenceService) Routines(ctx context.Context) ([]string, error) {
	cached, _, err := s.routinesCache.Get(ctx, "")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(cached))
	for name := range cached {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// RefreshRoutines forces a re-fetch on the next Routines/CallRoutine.
func (s *SequenceService) RefreshRoutines(ctx context.Context) error {
	s.routinesCache.InvalidateAll()
	_, _, err := s.routinesCache.Get(ctx, "")
	return err
}

// replaceRoutinePlaceholders walks obj recursively, substituting the
// per-call device identifiers for the placeholder strings upstream
// uses in stored routine bodies. Port of upstream
// utils.py replace_routine_placeholders.
func replaceRoutinePlaceholders(obj map[string]any, d *Device) map[string]any {
	for k, v := range obj {
		obj[k] = replaceRoutineValue(v, d)
	}
	return obj
}

func replaceRoutineValue(v any, d *Device) any {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			t[k] = replaceRoutineValue(vv, d)
		}
		return t
	case []any:
		for i, vv := range t {
			t[i] = replaceRoutineValue(vv, d)
		}
		return t
	case string:
		switch t {
		case "ALEXA_CURRENT_DSN",
			"<$Trigger.Alexa.Trigger.Alarms.NotificationStopped.$.payload.object.device.deviceSerialNumber$>":
			return d.SerialNumber
		case "ALEXA_CURRENT_DEVICE_TYPE",
			"<$Trigger.Alexa.Trigger.Alarms.NotificationStopped.$.payload.object.device.productId$>":
			return d.DeviceType
		}
		return t
	default:
		return t
	}
}

// deepCopyMap deep-clones a JSON-shaped map so callers can mutate
// without affecting the cached routine source.
func deepCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}
	return out
}

func deepCopyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return deepCopyMap(t)
	case []any:
		c := make([]any, len(t))
		for i, vv := range t {
			c[i] = deepCopyValue(vv)
		}
		return c
	default:
		return t
	}
}

// fetchRoutines is the loader for the sequence.routines cache. It calls
// GET /api/behaviors/v2/automations and returns the map of ENABLED
// routines keyed by name.
func (s *SequenceService) fetchRoutines(ctx context.Context) (map[string]map[string]any, error) {
	url := fmt.Sprintf("https://alexa.amazon.%s%s", s.wrapper.Domain(), amzconst.URIAutomations)
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method: http.MethodGet,
		URL:    url,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	if resp.StatusCode >= 400 {
		body := resp.Body
		defer body.Close()
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
	arr, _ := data[amzconst.ArrayWrapper].([]any)
	out := map[string]map[string]any{}
	for _, entry := range arr {
		row, _ := entry.(map[string]any)
		if row == nil {
			continue
		}
		status, _ := row["status"].(string)
		if status != "ENABLED" {
			continue
		}
		name, _ := row["name"].(string)
		sequence, _ := row["sequence"].(map[string]any)
		if name == "" || sequence == nil {
			continue
		}
		out[name] = sequence
	}
	return out, nil
}
