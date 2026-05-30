package alexactl

import "time"

// VocalRecord is the latest utterance captured by Alexa for a single
// device, returned by HistoryService.Vocal. Mirrors the per-device
// latest-record output of upstream get_vocal_history.
type VocalRecord struct {
	// Timestamp is the time Amazon recorded the utterance, in UTC.
	Timestamp time.Time

	// UtteranceType is Amazon's classification string (e.g.,
	// "GENERAL", "ROUTINES_OR_TAP_TO_ALEXA"). The four upstream
	// "ignore" types (ASR_TIMEOUT, DEVICE_ARBITRATION, NO_EXPRESSED_INTENT,
	// WAKE_WORD_ONLY) are filtered out before construction.
	UtteranceType string

	// Intent is the Alexa intent that matched (may be empty for free-form
	// requests).
	Intent string

	// Title is Amazon's short description of the utterance.
	Title string

	// SubTitle is Amazon's longer description of the utterance.
	SubTitle string
}
