package amazondevices

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// DeviceVolumes returns the current per-device volume + mute state.
// Fresh snapshot — no internal cache. Returned map is keyed by device
// serial number; missing devices are simply absent.
func (s *MediaService) DeviceVolumes(ctx context.Context) (map[string]*VolumeState, error) {
	url := fmt.Sprintf("https://alexa.amazon.%s%s",
		s.wrapper.Domain(), amzconst.URIDeviceVolumes)
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
// snapshots for the whole account — so devices is the *filter*, not
// the query. If devices is empty, MediaStates returns an empty map
// without contacting Amazon.
func (s *MediaService) MediaStates(ctx context.Context, devices []*Device) (map[string]*MediaState, error) {
	if len(devices) == 0 {
		return map[string]*MediaState{}, nil
	}
	queryDevice := devices[0]
	url := fmt.Sprintf(
		"https://alexa.amazon.%s%s?deviceSerialNumber=%s&deviceType=%s",
		s.wrapper.Domain(), amzconst.URIMediaState,
		queryDevice.SerialNumber, queryDevice.DeviceType,
	)
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

	sessions := indexMediaSessions(data)
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
// Cached per MediaService for the configured TTL (default 1h, overridable
// via WithSoundsTTL). Zero or negative TTL disables caching. Concurrent
// callers serialize on the cache mutex; the second caller sees the
// freshly populated cache.
//
// The returned map is a defensive copy — mutating it does not affect the
// cached value.
func (s *MediaService) Sounds(ctx context.Context) (map[string]Sound, error) {
	s.soundsMu.Lock()
	defer s.soundsMu.Unlock()

	if s.soundsTTL > 0 && s.soundsCache != nil &&
		time.Since(s.soundsFetchedAt) < s.soundsTTL {
		return copySoundsMap(s.soundsCache), nil
	}

	fresh, err := s.fetchSounds(ctx)
	if err != nil {
		return nil, err
	}

	if s.soundsTTL > 0 {
		s.soundsCache = fresh
		s.soundsFetchedAt = time.Now()
	} else {
		s.soundsCache = nil
		s.soundsFetchedAt = time.Time{}
	}
	return copySoundsMap(fresh), nil
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
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
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

// MusicProviders returns the available music providers. Filters to
// those whose supportedProperties include Alexa.Music.PlaySearchPhrase
// AND whose availability is "AVAILABLE". DefaultProvider tracks the
// providerData.isDefaultMusicProvider flag.
func (s *MediaService) MusicProviders(ctx context.Context) (map[string]MusicProvider, error) {
	url := fmt.Sprintf("https://alexa.amazon.%s%s",
		s.wrapper.Domain(), amzconst.URIMusicProviders)
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
