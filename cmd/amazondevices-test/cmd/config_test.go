package cmd

import (
	"strings"
	"testing"

	amazondevices "github.com/Fishwaldo/goamazondevices"
)

func devs() []*amazondevices.Device {
	return []*amazondevices.Device{
		{SerialNumber: "G1", AccountName: "Kitchen"},
		{SerialNumber: "G2", AccountName: "Bedroom"},
		{SerialNumber: "G3", AccountName: "Bedroom"},
	}
}

func TestResolveDeviceRef_SerialMatch(t *testing.T) {
	got, err := resolveDeviceRef(devs(), "G2")
	if err != nil || got != "G2" {
		t.Fatalf("got %q,%v; want G2,nil", got, err)
	}
}

func TestResolveDeviceRef_UniqueName(t *testing.T) {
	got, err := resolveDeviceRef(devs(), "Kitchen")
	if err != nil || got != "G1" {
		t.Fatalf("got %q,%v; want G1,nil", got, err)
	}
}

func TestResolveDeviceRef_NoMatch(t *testing.T) {
	_, err := resolveDeviceRef(devs(), "Garage")
	if err == nil || !strings.Contains(err.Error(), "no device") {
		t.Fatalf("want no-device error, got %v", err)
	}
}

func TestResolveDeviceRef_Ambiguous(t *testing.T) {
	_, err := resolveDeviceRef(devs(), "Bedroom")
	if err == nil {
		t.Fatal("want ambiguity error")
	}
	if !strings.Contains(err.Error(), "G2") || !strings.Contains(err.Error(), "G3") {
		t.Fatalf("ambiguity error should list candidates, got %v", err)
	}
}
