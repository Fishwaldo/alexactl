package amazondevices

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

func TestDevice_Range_NilWhenNoRangeController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{"Alexa.PowerController"},
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Range("Apparatus.Humidity"))
}

func TestDevice_Range_NilWhenNoController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceRangeController},
	}
	assert.Nil(t, d.Range("Apparatus.Humidity"))
}

func TestDevice_Range_NilWhenNoEndpointID(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		AlexaInterfaces: []string{amzconst.InterfaceRangeController},
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Range("Apparatus.Humidity"))
}

func TestRangeControl_Set_StubReturnsNotImplementedAndWarns(t *testing.T) {
	logger, buf := newRangeToggleLogCapture(t, slog.LevelWarn)
	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: nil,
		Site:       "https://www.amazon.com",
		Logger:     logger,
	})
	require.NoError(t, err)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceRangeController},
		controller:      newSmartHomeClient(wr),
	}

	err = d.Range("Apparatus.Humidity").Set(context.Background(), 42.0)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrControllerNotImplemented)

	logLine := buf.String()
	assert.Contains(t, logLine, "level=WARN")
	assert.Contains(t, logLine, "Alexa.RangeController")
	assert.Contains(t, logLine, "https://github.com/Fishwaldo/goamazondevices/issues")
	assert.Contains(t, logLine, "setRangeValue")
	assert.Contains(t, logLine, "S-1")
	assert.Contains(t, logLine, "Apparatus.Humidity")
	// Interface name is surfaced as a structured slog key (in
	// addition to appearing in the free-text message) so log
	// aggregators can filter by interface=Alexa.RangeController.
	assert.Contains(t, logLine, "interface=Alexa.RangeController")
}

func TestRangeControl_Adjust_StubReturnsNotImplementedAndWarns(t *testing.T) {
	logger, buf := newRangeToggleLogCapture(t, slog.LevelWarn)
	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: nil,
		Site:       "https://www.amazon.com",
		Logger:     logger,
	})
	require.NoError(t, err)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceRangeController},
		controller:      newSmartHomeClient(wr),
	}

	err = d.Range("Apparatus.Humidity").Adjust(context.Background(), 5.0)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrControllerNotImplemented)
	assert.Contains(t, buf.String(), "adjustRangeValue")
}

// newRangeToggleLogCapture is a tiny in-memory slog capture used by
// the Range and Toggle stub tests. Lives here rather than in a
// shared helper file because it's only two callers.
func newRangeToggleLogCapture(t *testing.T, level slog.Level) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})), &buf
}
