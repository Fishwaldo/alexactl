package amzconst

// Volume range supported by Amazon devices.
const (
	VolumeMin = 0
	VolumeMax = 100
)

// MaxCustomerAccountRetries is how many times we'll retry the customer-ID
// lookup right after registration. Each attempt sleeps 2s.
const MaxCustomerAccountRetries = 3

// SequenceBatchDelay controls the debounce window for batching sequence
// operations into a single /api/behaviors/preview POST. Unused in Phase 1
// (commands issue synchronously); Phase 2 will wire this up.
const SequenceBatchDelaySeconds = 0.3
