// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package httpwrapper

import (
	"testing"
	"time"
)

// TestDefaultRetryBudgetIsTrimmed asserts the package-default retry tuning
// mirrors upstream's trimmed [0, 2, 5]-second schedule (v14.x #929): 2
// retries with a 2s-5s backoff window, instead of the old 6-retry,
// 1s-21s schedule.
func TestDefaultRetryBudgetIsTrimmed(t *testing.T) {
	if retryMaxAttempts != 2 {
		t.Errorf("retryMaxAttempts = %d, want 2 (upstream [0,2,5])", retryMaxAttempts)
	}
	if retryWaitMin != 2*time.Second {
		t.Errorf("retryWaitMin = %v, want 2s", retryWaitMin)
	}
	if retryWaitMax != 5*time.Second {
		t.Errorf("retryWaitMax = %v, want 5s", retryWaitMax)
	}
}
