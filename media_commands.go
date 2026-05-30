package amazondevices

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// PlaySound triggers an Alexa sound on each cluster member of d.
// soundID must be present in Sounds(ctx); unknown IDs return
// "amazondevices: sound %q not available" without contacting Amazon.
func (s *MediaService) PlaySound(ctx context.Context, d *Device, soundID string) error {
	if err := s.validateSound(ctx, soundID); err != nil {
		return err
	}
	return s.fanoutAndFlush(ctx, d, SeqSound, soundID, "")
}

// validateSound returns nil if soundID is present in Sounds(ctx).
// Tests may override via validateSoundFn.
func (s *MediaService) validateSound(ctx context.Context, soundID string) error {
	if s.validateSoundFn != nil {
		return s.validateSoundFn(ctx, soundID)
	}
	sounds, err := s.Sounds(ctx)
	if err != nil {
		return err
	}
	if _, ok := sounds[soundID]; !ok {
		return fmt.Errorf("amazondevices: sound %q not available", soundID)
	}
	return nil
}

// InfoSkills returns the canonical list of "Info" skill names accepted
// by InfoSkill (e.g. "Alexa.Weather.Play"). The slice is a fresh copy.
func (s *MediaService) InfoSkills() []string {
	out := make([]string, len(amzconst.AlexaInfoSkills))
	copy(out, amzconst.AlexaInfoSkills)
	return out
}

// TextCommand sends an arbitrary spoken-style command to each cluster
// member of d (e.g., "what time is it"). Same wire path as Speak but
// authored by amzn1.ask.1p.tellalexa.
func (s *MediaService) TextCommand(ctx context.Context, d *Device, text string) error {
	return s.fanoutAndFlush(ctx, d, SeqTextCommand, text, "")
}

// LaunchSkill launches a specific skill by name on each cluster member
// of d. skillName is the Alexa-skill identifier (e.g., "MySkill").
func (s *MediaService) LaunchSkill(ctx context.Context, d *Device, skillName string) error {
	return s.fanoutAndFlush(ctx, d, SeqLaunchSkill, skillName, "")
}

// PlayMusic asks each cluster member of d to play music from the named
// provider. providerID must be a known music provider — call
// MusicProviders() to enumerate. Unknown providers return an error
// without contacting Amazon.
func (s *MediaService) PlayMusic(ctx context.Context, d *Device, query, providerID string) error {
	if err := s.validateMusicProvider(ctx, providerID); err != nil {
		return err
	}
	return s.fanoutAndFlush(ctx, d, SeqMusic, query, providerID)
}

// validateMusicProvider returns nil if providerID is present in
// MusicProviders(). Tests may override via validateMusicProviderFn.
func (s *MediaService) validateMusicProvider(ctx context.Context, providerID string) error {
	if s.validateMusicProviderFn != nil {
		return s.validateMusicProviderFn(ctx, providerID)
	}
	providers, err := s.MusicProviders(ctx)
	if err != nil {
		return err
	}
	if _, ok := providers[providerID]; !ok {
		return fmt.Errorf("amazondevices: music provider %q not available", providerID)
	}
	return nil
}

// Control issues a media-playback control (Play/Pause/Next/Previous/
// Rewind/FastForward/Stop) against the device's cluster.
//
// Stop is special: upstream routes it through the Sequence pipeline
// (Alexa.DeviceControls.Stop) so that any in-flight Speak/Music ops
// are also halted as part of the same coalesced batch. All other
// commands POST directly to /api/np/command — no batching.
func (s *MediaService) Control(ctx context.Context, d *Device, cmd MediaCommand) error {
	if cmd == CmdStop {
		return s.fanoutAndFlush(ctx, d, SeqStop, "", "")
	}

	url := fmt.Sprintf(
		"https://alexa.amazon.%s%s?deviceSerialNumber=%s&deviceType=%s",
		s.wrapper.Domain(), amzconst.URIMediaControl, d.SerialNumber, d.DeviceType,
	)
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:   http.MethodPost,
		URL:      url,
		JSONBody: map[string]any{"type": string(cmd)},
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer func() { _ = resp.Body.Close() }()
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

// CallRoutine triggers a named routine via Sequence (lazy-loads the
// routines cache if empty). The single-device routine is fanned out
// per cluster member.
func (s *MediaService) CallRoutine(ctx context.Context, d *Device, name string) error {
	if _, err := s.sequence.Routines(ctx); err != nil {
		return err
	}
	return s.fanoutAndFlush(ctx, d, SeqRoutine, name, "")
}

// InfoSkill triggers one of the canonical Alexa info skills (Weather,
// Date, Time, ...). Unknown skill names are rejected without contacting
// Amazon.
func (s *MediaService) InfoSkill(ctx context.Context, d *Device, skill string) error {
	if !amzconst.IsAlexaInfoSkill(skill) {
		return fmt.Errorf("amazondevices: unrecognised info skill %q", skill)
	}
	return s.fanoutAndFlush(ctx, d, SequenceType(skill), "", "")
}
