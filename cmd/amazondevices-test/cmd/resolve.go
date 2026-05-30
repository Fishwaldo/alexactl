package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	amazondevices "github.com/Fishwaldo/goamazondevices"
)

// registerDeviceFlag adds the standard optional --device flag bound to p.
func registerDeviceFlag(c *cobra.Command, p *string) {
	c.Flags().StringVar(p, "device", "",
		"target device serial or name (overrides configured default-device)")
}

// resolveTarget picks the target serial from the --device flag (serial or
// name, resolved against the device list) or the persisted default-device.
func resolveTarget(deviceFlag string, s *Store, devices []*amazondevices.Device) (string, error) {
	if deviceFlag != "" {
		return resolveDeviceRef(devices, deviceFlag)
	}
	if v, ok := s.Get("default-device"); ok {
		return v, nil
	}
	return "", fmt.Errorf("no device: pass --device <serial-or-name> or run 'config set default-device <serial-or-name>'")
}

// resolveDevice fetches the device targeted by --device or the persisted
// default-device. It lists devices only when name resolution is needed.
func resolveDevice(ctx context.Context, deviceFlag string, client *amazondevices.Client, s *Store) (*amazondevices.Device, error) {
	var devices []*amazondevices.Device
	if deviceFlag != "" {
		var err error
		devices, err = client.Devices.List(ctx)
		if err != nil {
			return nil, fmt.Errorf("list devices: %w", err)
		}
	}
	serial, err := resolveTarget(deviceFlag, s, devices)
	if err != nil {
		return nil, err
	}
	return client.Devices.Get(ctx, serial)
}

// resolveFilter turns an optional --device filter value (serial or name)
// into a concrete serial for list/filter commands. An empty flag yields
// an empty serial, meaning "all devices" — unlike resolveDevice/
// resolveTarget it never falls back to the configured default-device. A
// name is resolved against the live device list.
func resolveFilter(ctx context.Context, deviceFlag string, client *amazondevices.Client) (string, error) {
	if deviceFlag == "" {
		return "", nil
	}
	devices, err := client.Devices.List(ctx)
	if err != nil {
		return "", err
	}
	return resolveDeviceRef(devices, deviceFlag)
}
