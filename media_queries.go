package alexactl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/cache"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// DeviceVolumes returns the current per-device volume + mute state.
// Cached for the Realtime-class TTL. Returned map is a defensive copy.
func (s *MediaService) DeviceVolumes(ctx context.Context) (map[string]*VolumeState, error) {
	got, _, err := s.deviceVolumesCache.Get(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make(map[string]*VolumeState, len(got))
	for k, v := range got {
		cp := *v
		out[k] = &cp
	}
	return out, nil
}

// fetchDeviceVolumes is the loader for media.deviceVolumes.
func (s *MediaService) fetchDeviceVolumes(ctx context.Context) (map[string]*VolumeState, error) {
	url := fmt.Sprintf("https://alexa.amazon.%s%s",
		s.wrapper.Domain(), amzconst.URIDeviceVolumes)
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method: http.MethodGet,
		URL:    url,
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
	out := map[string]*VolumeState{}
	arr, _ := data["volumes"].([]any)
	for _, entry := range arr {
		row, _ := entry.(map[string]any)
		if row == nil {
			continue
		}
		dsn, _ := row["dsn"].(string)
		if dsn == "" {
			continue
		}
		vol, _ := numberAsInt64(row["speakerVolume"])
		muted, _ := row["speakerMuted"].(bool)
		out[dsn] = &VolumeState{Volume: int(vol), Muted: muted}
	}
	return out, nil
}

// MediaStates returns the current playback state for each device that
// has MediaPlayerSupported=true. The /api/np/list-media-sessions
// endpoint takes one (serial, type) as the query string but returns
// snapshots for the whole account — so the raw response is cached under
// sentinel key "" for the Realtime-class TTL, and the caller's devices
// list filters the cached snapshot on read. If devices is empty,
// MediaStates returns an empty map without contacting Amazon.
//
// The wire endpoint validates (serial, type) even though the response is
// account-wide, so the first caller's device is recorded and used as the
// loader's query parameters on subsequent cache misses.
func (s *MediaService) MediaStates(ctx context.Context, devices []*Device) (map[string]*MediaState, error) {
	if len(devices) == 0 {
		return map[string]*MediaState{}, nil
	}
	// Record any valid (serial, type) pair for the loader's wire query.
	// Prefer a MediaPlayerSupported device but fall back to the first
	// entry — Amazon validates these against the account, not the device's
	// capabilities.
	queryDevice := devices[0]
	for _, d := range devices {
		if d.MediaPlayerSupported {
			queryDevice = d
			break
		}
	}
	s.mediaStatesQueryMu.Lock()
	s.mediaStatesQuerySerial = queryDevice.SerialNumber
	s.mediaStatesQueryType = queryDevice.DeviceType
	s.mediaStatesQueryMu.Unlock()

	sessions, _, err := s.mediaStatesCache.Get(ctx, "")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := map[string]*MediaState{}
	for _, d := range devices {
		if !d.MediaPlayerSupported {
			continue
		}
		session, ok := sessions[d.SerialNumber]
		if !ok {
			continue
		}
		out[d.SerialNumber] = buildMediaState(session, now)
	}
	return out, nil
}

// fetchMediaStates is the loader for media.mediaStates. Uses the most
// recently recorded (serial, type) pair from MediaStates as the wire
// query — the response is account-wide regardless of the query device.
func (s *MediaService) fetchMediaStates(ctx context.Context) (map[string]map[string]any, error) {
	s.mediaStatesQueryMu.RLock()
	serial := s.mediaStatesQuerySerial
	devType := s.mediaStatesQueryType
	s.mediaStatesQueryMu.RUnlock()
	if serial == "" || devType == "" {
		return nil, fmt.Errorf("%w: media.mediaStates loader has no recorded query device — call MediaStates with at least one device first", ErrCannotRetrieveData)
	}
	url := fmt.Sprintf(
		"https://alexa.amazon.%s%s?deviceSerialNumber=%s&deviceType=%s",
		s.wrapper.Domain(), amzconst.URIMediaState,
		serial, devType,
	)
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method: http.MethodGet,
		URL:    url,
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
	return indexMediaSessions(data), nil
}

func indexMediaSessions(data map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	list, _ := data["mediaSessionList"].([]any)
	for _, entry := range list {
		session, _ := entry.(map[string]any)
		if session == nil {
			continue
		}
		endpoints, _ := session["endpointList"].([]any)
		for _, ep := range endpoints {
			row, _ := ep.(map[string]any)
			id, _ := row["id"].(map[string]any)
			serial, _ := id["deviceSerialNumber"].(string)
			if serial == "" {
				continue
			}
			out[serial] = session
		}
	}
	return out
}

func buildMediaState(session map[string]any, capturedAt time.Time) *MediaState {
	nowPlaying, _ := session["nowPlayingData"].(map[string]any)
	playerState, _ := nowPlaying["playerState"].(string)

	mainArt, _ := nowPlaying["mainArt"].(map[string]any)
	infoText, _ := nowPlaying["infoText"].(map[string]any)
	transport, _ := nowPlaying["transport"].(map[string]any)
	progress, _ := nowPlaying["progress"].(map[string]any)
	provider, _ := nowPlaying["provider"].(map[string]any)
	providerLogo, _ := provider["providerLogo"].(map[string]any)

	title, _ := infoText["title"].(string)
	sub1, _ := infoText["subText1"].(string)
	sub2, _ := infoText["subText2"].(string)
	nowURL, _ := mainArt["largeUrl"].(string)

	mediaLengthStr, _ := progress["mediaLength"].(string)
	mediaProgressStr, _ := progress["mediaProgress"].(string)

	ms := &MediaState{
		PlayerState:          playerState,
		NowPlayingURL:        nowURL,
		NowPlayingTitle:      title,
		NowPlayingLine1:      sub1,
		NowPlayingLine2:      sub2,
		NextEnabled:          transport["next"] == "ENABLED",
		PreviousEnabled:      transport["previous"] == "ENABLED",
		PauseEnabled:         transport["playPause"] == "ENABLED",
		SeekForwardEnabled:   transport["seekForward"] == "ENABLED",
		SeekBackEnabled:      transport["seekBack"] == "ENABLED",
		ShuffleEnabled:       transport["shuffle"] == "ENABLED",
		RepeatEnabled:        transport["repeat"] == "ENABLED",
		MediaPositionUpdated: capturedAt,
	}
	if mediaLengthStr != "" {
		if ms2, err := time.ParseDuration(mediaLengthStr + "ms"); err == nil {
			ms.MediaLength = ms2
		}
	}
	if mediaProgressStr != "" {
		if ms2, err := time.ParseDuration(mediaProgressStr + "ms"); err == nil {
			ms.MediaPosition = ms2
		}
	}
	pname, _ := provider["providerName"].(string)
	ms.MediaProvider = pname
	purl, _ := providerLogo["url"].(string)
	ms.MediaProviderURL = purl
	return ms
}

// Sounds returns the live Alexa sound-effects catalogue keyed by Sound.ID.
// Cached per MediaService for the configured Long-class TTL (overridable
// via WithSoundsTTL). Concurrent callers share the in-flight fetch.
//
// The returned map is a defensive copy.
func (s *MediaService) Sounds(ctx context.Context) (map[string]Sound, error) {
	got, _, err := s.soundsCache.Get(ctx, "")
	if err != nil {
		return nil, err
	}
	return copySoundsMap(got), nil
}

// marshalSoundsEntries / unmarshalSoundsEntries persist the
// media.sounds cache via the engine's Persistence adapter.
func marshalSoundsEntries(in map[string]cache.Entry[map[string]Sound]) ([]byte, error) {
	return json.Marshal(in)
}

func unmarshalSoundsEntries(b []byte) (map[string]cache.Entry[map[string]Sound], error) {
	out := map[string]cache.Entry[map[string]Sound]{}
	err := json.Unmarshal(b, &out)
	return out, err
}

// marshalMusicProvidersEntries / unmarshalMusicProvidersEntries persist
// the media.musicProviders cache via the engine's Persistence adapter.
func marshalMusicProvidersEntries(in map[string]cache.Entry[map[string]MusicProvider]) ([]byte, error) {
	return json.Marshal(in)
}

func unmarshalMusicProvidersEntries(b []byte) (map[string]cache.Entry[map[string]MusicProvider], error) {
	out := map[string]cache.Entry[map[string]MusicProvider]{}
	err := json.Unmarshal(b, &out)
	return out, err
}

func copySoundsMap(in map[string]Sound) map[string]Sound {
	out := make(map[string]Sound, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *MediaService) fetchSounds(ctx context.Context) (map[string]Sound, error) {
	url := fmt.Sprintf("https://alexa.amazon.%s%s",
		s.wrapper.Domain(), amzconst.URISounds)
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method: http.MethodGet,
		URL:    url,
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

	out := map[string]Sound{}
	arr, _ := data[amzconst.ArrayWrapper].([]any)
	for _, entry := range arr {
		row, _ := entry.(map[string]any)
		if row == nil {
			continue
		}
		if avail, _ := row["availability"].(string); avail != "AVAILABLE" {
			continue
		}
		id, _ := row["id"].(string)
		name, _ := row["displayName"].(string)
		if id == "" || name == "" {
			continue
		}
		pd, _ := row["providerData"].(map[string]any)
		cat, _ := pd["categoryType"].(string)
		catName, _ := pd["categoryDisplayName"].(string)
		out[id] = Sound{
			ID:                  id,
			DisplayName:         name,
			Category:            cat,
			CategoryDisplayName: catName,
		}
	}
	return out, nil
}

// MusicProviders returns the available music providers. Cached for the
// Long-class TTL and tagged "session" — account switches drop the cache
// automatically. Returned map is a defensive copy.
func (s *MediaService) MusicProviders(ctx context.Context) (map[string]MusicProvider, error) {
	got, _, err := s.musicProvidersCache.Get(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make(map[string]MusicProvider, len(got))
	for k, v := range got {
		out[k] = v
	}
	return out, nil
}

// fetchMusicProviders is the loader for media.musicProviders. Filters to
// providers whose supportedProperties include Alexa.Music.PlaySearchPhrase
// AND whose availability is "AVAILABLE". DefaultProvider tracks the
// providerData.isDefaultMusicProvider flag.
func (s *MediaService) fetchMusicProviders(ctx context.Context) (map[string]MusicProvider, error) {
	url := fmt.Sprintf("https://alexa.amazon.%s%s",
		s.wrapper.Domain(), amzconst.URIMusicProviders)
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method: http.MethodGet,
		URL:    url,
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
	out := map[string]MusicProvider{}
	arr, _ := data[amzconst.ArrayWrapper].([]any)
	for _, entry := range arr {
		row, _ := entry.(map[string]any)
		if row == nil {
			continue
		}
		props, _ := row["supportedProperties"].([]any)
		supportsMusic := false
		for _, p := range props {
			if s2, ok := p.(string); ok && s2 == string(SeqMusic) {
				supportsMusic = true
				break
			}
		}
		if !supportsMusic {
			continue
		}
		availability, _ := row["availability"].(string)
		if availability != "AVAILABLE" {
			continue
		}
		id, _ := row["id"].(string)
		name, _ := row["displayName"].(string)
		if id == "" || name == "" {
			continue
		}
		providerData, _ := row["providerData"].(map[string]any)
		isDefault, _ := providerData["isDefaultMusicProvider"].(bool)
		out[id] = MusicProvider{
			ProviderID:      id,
			ProviderName:    name,
			Availability:    availability,
			DefaultProvider: isDefault,
		}
	}
	return out, nil
}
