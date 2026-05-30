package alexactl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDeviceDetails(t *testing.T) {
	cases := []struct {
		in        string
		wantModel string
		wantHW    string
	}{
		{"", "", ""},
		{"Echo Dot (5th Gen)", "Echo Dot", "5th Gen"},
		{"Echo Dot (5th gen) with Clock", "Echo Dot with Clock", "5th Gen"},
		{"Fire TV Stick 4K (2nd Generation)", "Fire TV Stick 4K", "2nd Gen"},
		{"2021 Samsung UHD TV", "Samsung UHD TV", "2021"},
		{"Panasonic Viera (2019)", "Panasonic Viera", "2019"},
		{"Echo Show", "Echo Show", ""},
	}
	for _, tc := range cases {
		gotModel, gotHW := parseDeviceDetails(tc.in)
		assert.Equal(t, tc.wantModel, gotModel, "in=%q", tc.in)
		assert.Equal(t, tc.wantHW, gotHW, "in=%q", tc.in)
	}
}
