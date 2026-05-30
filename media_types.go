package alexactl

import "time"

// MediaState is a per-device snapshot of the current /api/np/list-media-sessions
// entry. Returned by MediaService.MediaStates. Fields with no wire value
// are zero-valued strings/bools/durations/time.Time — Go's idiom rather
// than Python's Optional.
type MediaState struct {
	PlayerState          string // "PLAYING" | "PAUSED" | "IDLE" | ""
	NowPlayingURL        string // mainArt.largeUrl
	NowPlayingTitle      string // infoText.title
	NowPlayingLine1      string // infoText.subText1
	NowPlayingLine2      string // infoText.subText2
	NextEnabled          bool
	PreviousEnabled      bool
	PauseEnabled         bool
	SeekForwardEnabled   bool
	SeekBackEnabled      bool
	ShuffleEnabled       bool
	RepeatEnabled        bool
	MediaLength          time.Duration // 0 if absent
	MediaPosition        time.Duration // 0 if absent
	MediaPositionUpdated time.Time     // local capture time
	MediaProvider        string        // provider.providerName
	MediaProviderURL     string        // provider.providerLogo.url
}

// VolumeState is the per-device volume snapshot returned by
// MediaService.DeviceVolumes.
type VolumeState struct {
	Volume int // 0..100
	Muted  bool
}

// MusicProvider is one entry from MediaService.MusicProviders. Only
// providers that support Alexa.Music.PlaySearchPhrase AND have an
// availability of "AVAILABLE" are surfaced.
type MusicProvider struct {
	ProviderID      string
	ProviderName    string
	Availability    string
	DefaultProvider bool
}

// Sound is a single Alexa sound effect from the routine sound catalogue.
// Returned by MediaService.Sounds(ctx). Only entries with availability
// "AVAILABLE" on the wire are surfaced.
type Sound struct {
	ID                  string // wire id, pass to PlaySound (e.g. "amzn_sfx_doorbell_01")
	DisplayName         string // localized label (e.g. "Doorbell 1")
	Category            string // wire category token (e.g. "bell_and_buzzers")
	CategoryDisplayName string // localized category label (e.g. "Bells & Buzzers")
}
