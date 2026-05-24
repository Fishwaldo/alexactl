package amazondevices

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAPIError_UnwrapMatchesCategory(t *testing.T) {
	apiErr := &APIError{
		Category:   ErrCannotAuthenticate,
		Method:     "POST",
		StatusCode: 401,
		URL:        "https://www.amazon.com/ap/signin",
	}

	var err error = apiErr

	assert.True(t, errors.Is(err, ErrCannotAuthenticate),
		"APIError must unwrap to its Category sentinel")
	assert.False(t, errors.Is(err, ErrCannotConnect))

	var got *APIError
	assert.True(t, errors.As(err, &got))
	assert.Equal(t, 401, got.StatusCode)
	assert.Equal(t, "POST", got.Method)
}

func TestAPIError_ErrorString(t *testing.T) {
	apiErr := &APIError{
		Category:   ErrCannotRetrieveData,
		Method:     "GET",
		StatusCode: 503,
		URL:        "https://example.com/x",
	}

	got := apiErr.Error()
	assert.Contains(t, got, "503")
	assert.Contains(t, got, "https://example.com/x")
	assert.Contains(t, got, "GET")
	assert.Contains(t, got, ErrCannotRetrieveData.Error())
}

// TestAPIError_ErrorString_EmptyMethodAndStatus covers the documented case
// where the failure was not tied to a specific HTTP call: a zero Method and
// StatusCode must not surface as " 0 " noise.
func TestAPIError_ErrorString_EmptyMethodAndStatus(t *testing.T) {
	apiErr := &APIError{
		Category: ErrCannotRetrieveData,
		URL:      "https://example.com/x",
	}

	got := apiErr.Error()
	assert.Equal(t, "amazondevices: cannot retrieve data: https://example.com/x", got)
	assert.NotContains(t, got, " 0 ")
}

// TestAPIError_ErrorString_OnlyCategory covers the case where neither
// Method, StatusCode, nor URL is populated — the category alone should
// render with no trailing punctuation or whitespace.
func TestAPIError_ErrorString_OnlyCategory(t *testing.T) {
	apiErr := &APIError{Category: ErrCannotRetrieveData}

	got := apiErr.Error()
	assert.Equal(t, ErrCannotRetrieveData.Error(), got)
}

// TestAPIError_ErrorString_StatusButNoMethod covers a pre-response failure
// edge — if a caller sets a status but no method (or vice versa), we still
// render the HTTP-shaped form, since at least one HTTP-level field is set.
func TestAPIError_ErrorString_StatusButNoMethod(t *testing.T) {
	apiErr := &APIError{
		Category:   ErrCannotConnect,
		StatusCode: 0,
		Method:     "POST",
		URL:        "https://example.com/y",
	}

	got := apiErr.Error()
	assert.Contains(t, got, "POST")
	assert.Contains(t, got, "https://example.com/y")
	assert.Contains(t, got, ErrCannotConnect.Error())
}

func TestSentinelsAreDistinct(t *testing.T) {
	all := []error{
		ErrCannotAuthenticate,
		ErrCannotConnect,
		ErrCannotRetrieveData,
		ErrCannotRegisterDevice,
		ErrWrongMethod,
		ErrReauthRequired,
	}
	for i, a := range all {
		for j, b := range all {
			if i == j {
				continue
			}
			assert.NotErrorIs(t, a, b, "%v should not match %v", a, b)
		}
	}
}
