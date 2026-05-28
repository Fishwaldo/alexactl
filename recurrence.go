package amazondevices

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Recurrence wraps an RFC 5545 RRULE describing how an alarm or
// reminder repeats. Construct via the package-level helpers below
// (preferred for common cases) or RecurrenceRule for custom RRULE
// strings.
//
// At Create/Update time the service layer appends BYHOUR / BYMINUTE /
// BYSECOND to the stored RRULE using the alarm/reminder's FireAt
// time-of-day, then sends it under rRuleData.recurrenceRules. Callers
// don't see this — the public surface is just the helpers below.
type Recurrence struct {
	rrule string
}

// RecurrenceDaily fires every day at the alarm/reminder's FireAt time.
func RecurrenceDaily() *Recurrence {
	return &Recurrence{rrule: "FREQ=DAILY"}
}

// RecurrenceWeekdays fires every Monday through Friday.
func RecurrenceWeekdays() *Recurrence {
	return &Recurrence{rrule: "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"}
}

// RecurrenceWeekends fires every Saturday and Sunday.
func RecurrenceWeekends() *Recurrence {
	return &Recurrence{rrule: "FREQ=WEEKLY;BYDAY=SA,SU"}
}

// RecurrenceWeekly fires on the given days of the week. Days are
// deduplicated and emitted in canonical Mon-first order regardless of
// input order. At least one day must be supplied.
func RecurrenceWeekly(days ...time.Weekday) *Recurrence {
	seen := make(map[time.Weekday]struct{}, len(days))
	var unique []time.Weekday
	for _, d := range days {
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		unique = append(unique, d)
	}
	sort.Slice(unique, func(i, j int) bool {
		// time.Sunday = 0, time.Monday = 1, ... time.Saturday = 6.
		// Reorder so Monday is first (Mon=0, Tue=1, ..., Sun=6).
		return mondayFirst(unique[i]) < mondayFirst(unique[j])
	})
	parts := make([]string, 0, len(unique))
	for _, d := range unique {
		parts = append(parts, weekdayRRule(d))
	}
	return &Recurrence{rrule: "FREQ=WEEKLY;BYDAY=" + strings.Join(parts, ",")}
}

// RecurrenceRule is the raw-RRULE escape hatch for cases the typed
// helpers don't cover (monthly, custom intervals, etc.). The caller
// is responsible for the RRULE's correctness.
func RecurrenceRule(rrule string) *Recurrence {
	return &Recurrence{rrule: rrule}
}

// RRule returns the canonical RFC 5545 RRULE string.
func (r *Recurrence) RRule() string {
	if r == nil {
		return ""
	}
	return r.rrule
}

// Describe returns a short human-friendly description of the
// recurrence. Used by the CLI for display; the library doesn't
// otherwise depend on the format.
func (r *Recurrence) Describe() string {
	if r == nil {
		return "One-shot"
	}
	switch r.rrule {
	case "FREQ=DAILY":
		return "Daily"
	case "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR":
		return "Weekdays (Mon-Fri)"
	case "FREQ=WEEKLY;BYDAY=SA,SU":
		return "Weekends (Sat-Sun)"
	}
	if strings.HasPrefix(r.rrule, "FREQ=WEEKLY;BYDAY=") {
		days := strings.TrimPrefix(r.rrule, "FREQ=WEEKLY;BYDAY=")
		parts := strings.Split(days, ",")
		long := make([]string, len(parts))
		for i, p := range parts {
			long[i] = rruleToShortName(p)
		}
		return "Weekly on " + strings.Join(long, ", ")
	}
	return "Custom: " + r.rrule
}

// mondayFirst maps time.Weekday to a 0=Monday ordering.
func mondayFirst(d time.Weekday) int {
	// time.Sunday=0 → 6; Monday=1 → 0; ...; Saturday=6 → 5.
	if d == time.Sunday {
		return 6
	}
	return int(d) - 1
}

// weekdayRRule renders a time.Weekday as its RFC 5545 two-letter code.
func weekdayRRule(d time.Weekday) string {
	return [...]string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}[int(d)]
}

// rruleToShortName converts "MO" → "Mon", "TU" → "Tue", etc.
func rruleToShortName(code string) string {
	switch code {
	case "MO":
		return "Mon"
	case "TU":
		return "Tue"
	case "WE":
		return "Wed"
	case "TH":
		return "Thu"
	case "FR":
		return "Fri"
	case "SA":
		return "Sat"
	case "SU":
		return "Sun"
	}
	return code
}

// buildFullRRule appends BYHOUR / BYMINUTE / BYSECOND / INTERVAL
// derived from the given FireAt time, producing the final RRULE
// string Amazon expects. Returns the canonical RRULE if the
// Recurrence already includes BY* fields (Path A trust the caller's
// RecurrenceRule escape hatch). Used by the service body-builders.
func (r *Recurrence) buildFullRRule(fireAt time.Time) string {
	if r == nil {
		return ""
	}
	if strings.Contains(r.rrule, "BYHOUR=") {
		return r.rrule
	}
	hour, min, sec := fireAt.Clock()
	tail := fmt.Sprintf(";BYHOUR=%d;BYMINUTE=%d;BYSECOND=%d;INTERVAL=1", hour, min, sec)
	return r.rrule + tail
}
