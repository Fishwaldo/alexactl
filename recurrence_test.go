package amazondevices

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRecurrenceDaily_GeneratesExpectedRRule(t *testing.T) {
	r := RecurrenceDaily()
	assert.Equal(t, "FREQ=DAILY", r.RRule())
}

func TestRecurrenceWeekdays_GeneratesExpectedRRule(t *testing.T) {
	r := RecurrenceWeekdays()
	assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", r.RRule())
}

func TestRecurrenceWeekends_GeneratesExpectedRRule(t *testing.T) {
	r := RecurrenceWeekends()
	assert.Equal(t, "FREQ=WEEKLY;BYDAY=SA,SU", r.RRule())
}

func TestRecurrenceWeekly_OrdersDaysCanonically(t *testing.T) {
	// Input out of order; output must be Mon < Tue < Wed
	r := RecurrenceWeekly(time.Wednesday, time.Monday, time.Tuesday)
	assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO,TU,WE", r.RRule())
}

func TestRecurrenceWeekly_DeduplicatesRepeatedDays(t *testing.T) {
	r := RecurrenceWeekly(time.Monday, time.Monday, time.Friday)
	assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO,FR", r.RRule())
}

func TestRecurrenceRule_PassesThroughVerbatim(t *testing.T) {
	raw := "FREQ=MONTHLY;BYMONTHDAY=1;BYHOUR=9;BYMINUTE=0"
	r := RecurrenceRule(raw)
	assert.Equal(t, raw, r.RRule())
}

func TestRecurrence_Describe_HumanFriendly(t *testing.T) {
	tests := []struct {
		name     string
		rec      *Recurrence
		expected string
	}{
		{"daily", RecurrenceDaily(), "Daily"},
		{"weekdays", RecurrenceWeekdays(), "Weekdays (Mon-Fri)"},
		{"weekends", RecurrenceWeekends(), "Weekends (Sat-Sun)"},
		{"weekly-tue", RecurrenceWeekly(time.Tuesday), "Weekly on Tue"},
		{"weekly-mwf", RecurrenceWeekly(time.Monday, time.Wednesday, time.Friday), "Weekly on Mon, Wed, Fri"},
		{"custom", RecurrenceRule("FREQ=MONTHLY;BYMONTHDAY=1"), "Custom: FREQ=MONTHLY;BYMONTHDAY=1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.rec.Describe())
		})
	}
}

func TestRecurrence_RRule_NilReceiverReturnsEmpty(t *testing.T) {
	var r *Recurrence
	assert.Equal(t, "", r.RRule())
}

func TestRecurrence_Describe_NilReceiverReturnsOneShot(t *testing.T) {
	var r *Recurrence
	assert.Equal(t, "One-shot", r.Describe())
}

func TestBuildFullRRule_AppendsBYHourMinuteSecondFromFireAt(t *testing.T) {
	r := RecurrenceWeekdays()
	fireAt := time.Date(2026, 5, 29, 7, 30, 15, 0, time.UTC)
	got := r.buildFullRRule(fireAt)
	assert.Equal(t,
		"FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR;BYHOUR=7;BYMINUTE=30;BYSECOND=15;INTERVAL=1",
		got)
}

func TestBuildFullRRule_SkipsAppendWhenRRuleAlreadyHasBYHour(t *testing.T) {
	// Custom RRULE supplies its own BYHOUR — buildFullRRule must
	// trust the caller and pass through verbatim.
	raw := "FREQ=MONTHLY;BYMONTHDAY=1;BYHOUR=9;BYMINUTE=0"
	r := RecurrenceRule(raw)
	fireAt := time.Date(2026, 5, 29, 7, 30, 0, 0, time.UTC) // different time
	assert.Equal(t, raw, r.buildFullRRule(fireAt),
		"buildFullRRule must not append when BYHOUR is already present in RRULE")
}

func TestBuildFullRRule_NilReceiverReturnsEmpty(t *testing.T) {
	var r *Recurrence
	assert.Equal(t, "", r.buildFullRRule(time.Now()))
}
