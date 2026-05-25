package amzconst

// Notification type names returned by Amazon's /api/notifications endpoint.
// Values are exact wire strings.
const (
	NotificationAlarm      = "Alarm"
	NotificationMusicAlarm = "MusicAlarm"
	NotificationReminder   = "Reminder"
	NotificationTimer      = "Timer"
)

// NotificationsSupported is the set of notification types we surface.
// MusicAlarm is normalized to Alarm at parse time (same shape, different label).
var NotificationsSupported = map[string]struct{}{
	NotificationAlarm:      {},
	NotificationMusicAlarm: {},
	NotificationReminder:   {},
	NotificationTimer:      {},
}

// RecurringPatterns is the canonical mapping from Amazon's compact rule
// shorthand (XXXX-WD = weekdays, XXXX-WE = weekend, XXXX-WXX-N = weekly on
// weekday N) to its RFC 5545 expansion. Port of upstream
// const/schedules.py RECURRING_PATTERNS.
//
// Country-specific weekend redefinitions are overlayed via WeekendExceptions.
var RecurringPatterns = map[string]string{
	"P1D":        "FREQ=DAILY",
	"XXXX-WD":    "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR",
	"XXXX-WE":    "FREQ=WEEKLY;BYDAY=SA,SU",
	"XXXX-WXX-1": "FREQ=WEEKLY;BYDAY=MO",
	"XXXX-WXX-2": "FREQ=WEEKLY;BYDAY=TU",
	"XXXX-WXX-3": "FREQ=WEEKLY;BYDAY=WE",
	"XXXX-WXX-4": "FREQ=WEEKLY;BYDAY=TH",
	"XXXX-WXX-5": "FREQ=WEEKLY;BYDAY=FR",
	"XXXX-WXX-6": "FREQ=WEEKLY;BYDAY=SA",
	"XXXX-WXX-7": "FREQ=WEEKLY;BYDAY=SU",
}

// WeekendExceptions overrides the WD/WE patterns for countries whose
// weekend differs from Saturday/Sunday. Keyed by weekend-type group; the
// inner map is merged into a per-call copy of RecurringPatterns.
//
//	"TH-FR" — Thursday–Friday weekend (e.g., Iran)
//	"FR-SA" — Friday–Saturday weekend (e.g., Saudi Arabia, Egypt)
var WeekendExceptions = map[string]map[string]string{
	"TH-FR": {
		"XXXX-WD": "FREQ=WEEKLY;BYDAY=MO,TU,WE,SA,SU",
		"XXXX-WE": "FREQ=WEEKLY;BYDAY=TH,FR",
	},
	"FR-SA": {
		"XXXX-WD": "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,SU",
		"XXXX-WE": "FREQ=WEEKLY;BYDAY=FR,SA",
	},
}

// CountryGroups groups ISO-3166 alpha-2 country codes by weekend type.
// Used to select the right WeekendExceptions overlay for a session's
// country. Codes match what httpwrapper.Wrapper.CountryCode returns
// (lowercase, two letters).
var CountryGroups = map[string][]string{
	"TH-FR": {"ir"},
	"FR-SA": {
		"af", "bd", "bh", "dz", "eg", "il", "iq", "jo", "kw", "ly",
		"mv", "my", "om", "ps", "qa", "sa", "sd", "sy", "ye",
	},
}

// Capability strings used to filter notification types onto devices.
// Mirrors the per-type capability filter in upstream sensor.py
// _update_sensor_data.
const (
	CapabilityReminders       = "REMINDERS"
	CapabilityTimersAndAlarms = "TIMERS_AND_ALARMS"
)
