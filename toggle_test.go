package alexactl

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

func TestDevice_Toggle_NilWhenNoToggleController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{"Alexa.PowerController"},
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Toggle("Skill.Ionizer"))
}

func TestDevice_Toggle_NilWhenNoController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceToggleController},
	}
	assert.Nil(t, d.Toggle("Skill.Ionizer"))
}

func TestDevice_Toggle_NilWhenNoEndpointID(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		AlexaInterfaces: []string{amzconst.InterfaceToggleController},
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Toggle("Skill.Ionizer"))
}

func TestToggleControl_On_StubReturnsNotImplementedAndWarns(t *testing.T) {
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
		AlexaInterfaces: []string{amzconst.InterfaceToggleController},
		controller:      newSmartHomeClient(wr),
	}

	err = d.Toggle("Skill.Ionizer").On(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrControllerNotImplemented)

	logLine := buf.String()
	assert.Contains(t, logLine, "level=WARN")
	assert.Contains(t, logLine, "Alexa.ToggleController")
	assert.Contains(t, logLine, "https://github.com/Fishwaldo/alexactl/issues")
	assert.Contains(t, logLine, "turnOn")
	assert.Contains(t, logLine, "S-1")
	assert.Contains(t, logLine, "Skill.Ionizer")
	// Interface name surfaced as a structured slog key for
	// aggregator filtering.
	assert.Contains(t, logLine, "interface=Alexa.ToggleController")
}

func TestToggleControl_Off_StubReturnsNotImplementedAndWarns(t *testing.T) {
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
		AlexaInterfaces: []string{amzconst.InterfaceToggleController},
		controller:      newSmartHomeClient(wr),
	}

	err = d.Toggle("Skill.Ionizer").Off(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrControllerNotImplemented)
	assert.Contains(t, buf.String(), "turnOff")
}
