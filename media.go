package alexactl

import (
	"context"
	"fmt"
	"sync"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/cache"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// MediaService is the user-facing surface for Alexa media operations.
// Command facades (Speak/Announce/SetVolume/PlaySound/PlayMusic/...)
// route through SequenceService for batching + ParallelNode wrapping.
// State queries (DeviceVolumes/MediaStates/MusicProviders) call
// alexa.amazon.<domain> directly.
//
// Safe for concurrent use.
type MediaService struct {
	wrapper *httpwrapper.Wrapper

	// sequence is the SequenceService instance owned by the same Client.
	// Wired by NewClient at construction time. Tests must wire it
	// explicitly when constructing MediaService bare.
	sequence *SequenceService

	mu      sync.RWMutex
	session *Session

	// validateMusicProviderFn is a test seam for PlayMusic's provider
	// lookup; production leaves nil and PlayMusic calls MusicProviders()
	// directly. Populated by media_commands_test.go for unit isolation.
	validateMusicProviderFn func(ctx context.Context, providerID string) error

	// validateSoundFn is a test seam for PlaySound's catalogue check;
	// production leaves nil and PlaySound calls Sounds() directly.
	validateSoundFn func(ctx context.Context, soundID string) error

	// soundsCache is the engine-managed sounds catalogue cache. Constructed
	// by NewClient (or by test helpers for unit tests). Nil means uncached.
	soundsCache *cache.Cache[map[string]Sound]

	// musicProvidersCache caches MusicProviders() for the Long-class TTL,
	// tagged "session". Constructed by NewClient.
	musicProvidersCache *cache.Cache[map[string]MusicProvider]

	// deviceVolumesCache caches DeviceVolumes() for the Realtime-class TTL.
	// Account-wide endpoint; sentinel key "".
	deviceVolumesCache *cache.Cache[map[string]*VolumeState]

	// mediaStatesCache caches the raw account-wide media-sessions response
	// (indexed by serial) for the Realtime-class TTL. MediaStates applies
	// the caller's device filter on read. Sentinel key "".
	mediaStatesCache *cache.Cache[map[string]map[string]any]

	// mediaStatesQuerySerial / mediaStatesQueryType hold the last caller-
	// supplied device used as the wire query params for the media-sessions
	// endpoint. Amazon rejects requests without a real (serial, type) pair
	// even though the response is account-wide. Updated by MediaStates on
	// every call; read by the cache loader.
	mediaStatesQueryMu     sync.RWMutex
	mediaStatesQuerySerial string
	mediaStatesQueryType   string
}

// setSession swaps in a fresh *Session pointer. Invoked by AuthService
// via the broadcast pipeline. Also invalidates the sounds catalogue
// cache so a locale change after account switch refetches localized
// labels.
func (s *MediaService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
	if s.soundsCache != nil {
		s.soundsCache.InvalidateAll()
	}
}

// snapshotSession returns the current *Session pointer under the read
// lock.
func (s *MediaService) snapshotSession() *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.session
}

// Speak utters text on the given device (and on each cluster member).
// One Sequence.Send per cluster member is buffered and a single Flush
// dispatches them in a single POST (optimised into a ParallelNode by
// the optimisation pass).
func (s *MediaService) Speak(ctx context.Context, d *Device, text string) error {
	return s.fanoutAndFlush(ctx, d, SeqSpeak, text, "")
}

// Announce broadcasts a message on the device's cluster. Cluster
// fanout lives inside the operation payload's target.devices field
// (not as per-member nodes), so this is a single Send + Flush.
func (s *MediaService) Announce(ctx context.Context, d *Device, text string) error {
	if err := s.sequence.Send(ctx, d, SeqAnnouncement, text, ""); err != nil {
		return err
	}
	return s.sequence.Flush(ctx)
}

// SetVolume sets a volume in [0, 100] on each cluster member.
func (s *MediaService) SetVolume(ctx context.Context, d *Device, vol int) error {
	if vol < amzconst.VolumeMin || vol > amzconst.VolumeMax {
		return fmt.Errorf("alexactl: volume %d not in [%d,%d]",
			vol, amzconst.VolumeMin, amzconst.VolumeMax)
	}
	return s.fanoutAndFlush(ctx, d, SeqVolume, vol, "")
}

// fanoutAndFlush enqueues one Sequence.Send per cluster member (or
// for the device itself if it has no cluster), then Flushes so the
// resulting batch is POSTed synchronously.
func (s *MediaService) fanoutAndFlush(ctx context.Context, d *Device, kind SequenceType, body any, providerID string) error {
	members := sortedKeys(d.ClusterMembers)
	if len(members) == 0 {
		members = []string{d.SerialNumber}
	}
	for _, serial := range members {
		dt := d.ClusterMembers[serial]
		if dt == "" {
			dt = d.DeviceType
		}
		fanout := &Device{SerialNumber: serial, DeviceType: dt}
		if err := s.sequence.Send(ctx, fanout, kind, body, providerID); err != nil {
			return err
		}
	}
	return s.sequence.Flush(ctx)
}
