# alexactl

[![CI](https://github.com/Fishwaldo/alexactl/actions/workflows/ci.yml/badge.svg)](https://github.com/Fishwaldo/alexactl/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/Fishwaldo/alexactl.svg)](https://pkg.go.dev/github.com/Fishwaldo/alexactl)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A Go library for controlling Amazon Echo / Alexa devices and skill-discovered
smart-home endpoints (lights, switches, locks, thermostats, fans, …).

> **Status: v0.1.0 — MVP.** Mirrors functionality of the Python library
> [aioamazondevices](https://github.com/chemelli74/aioamazondevices). The
> API is still stabilizing pre-1.0; expect breaking changes in `MINOR`
> releases. See `CHANGELOG.md` for per-release detail.

## Install

```bash
go get github.com/Fishwaldo/alexactl
```

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Fishwaldo/alexactl"
)

func main() {
    ctx := context.Background()

    client, err := alexactl.NewClient()
    if err != nil {
        log.Fatal(err)
    }

    session, err := client.Auth.Login(ctx, "you@example.com", "your-password",
        alexactl.OTPProviderFunc(func(ctx context.Context) (string, error) {
            var otp string
            fmt.Print("Enter Amazon OTP: ")
            fmt.Scanln(&otp)
            return otp, nil
        }))
    if err != nil {
        log.Fatal(err)
    }
    _ = session // persist to disk for next run

    devices, err := client.Devices.List(ctx)
    if err != nil {
        log.Fatal(err)
    }
    for _, d := range devices {
        fmt.Printf("%s (%s, online=%v)\n", d.AccountName, d.SerialNumber, d.Online)
    }
}
```

### Smart-home control

Capability-gated typed accessors on `*Device` return `nil` if the device
does not advertise the corresponding Alexa interface — callers write
capability-conditional code without sniffing strings:

```go
for _, d := range devices {
    if pc := d.Power(); pc != nil {
        _ = pc.On(ctx)
    }
    if bc := d.Brightness(); bc != nil {
        _ = bc.Set(ctx, 80) // 0..100
    }
    if cc := d.Color(); cc != nil {
        _ = cc.Set(ctx, alexactl.ColorFromRGB(255, 64, 0))
    }
    if lc := d.Lock(); lc != nil {
        _ = lc.Lock(ctx)
    }
    if tc := d.Thermostat(); tc != nil {
        _ = tc.SetTarget(ctx, 21, alexactl.TemperatureScaleCelsius)
    }
    if mc := d.Mode("Apparatus.FanSpeed"); mc != nil {
        _ = mc.Set(ctx, "L3")
    }
}
```

See `cmd/alexactl/` for a full CLI example.

## CLI test harness

`cmd/alexactl/` is a Cobra/Viper CLI mirroring the Python
`library_test.py`. Build and use:

```bash
go build -o alexactl ./cmd/alexactl

# Interactive login (prompts for OTP)
./alexactl login --email you@example.com --password 'your-password'

# Inventory + state
./alexactl devices list
./alexactl devices list --detail
./alexactl sensors list

# Media (Echo speakers). Every single-device command takes --device
# (serial or friendly name); omit it to use the configured default-device.
./alexactl speak    "hello"        --device "Kitchen"
./alexactl announce "dinner ready" --device "Kitchen"
./alexactl volume   30             --device "Kitchen"
./alexactl music    AMAZON_MUSIC "miles davis" --device "Kitchen"
./alexactl sound play amzn_sfx_doorbell_01     --device "Kitchen"
./alexactl dnd set  on             --device "Kitchen"

# Smart-home control
./alexactl devices power      on --device "Lamp"
./alexactl devices brightness 80 --device "Lamp"
./alexactl devices color      ff4000 --device "Lamp"
./alexactl devices lock       --device "Front Door"
./alexactl devices thermostat set-target 21 c --device "Hallway"
./alexactl devices mode set   Apparatus.FanSpeed L3 --device "AC"
```

Configuration can also be supplied via `./config.yaml`:

```yaml
email: you@example.com
password: your-password
session_file: session.json
site: https://www.amazon.com
```

Environment variables use the `AMZ_` prefix (e.g., `AMZ_EMAIL`, `AMZ_PASSWORD`).

## Feature parity vs aioamazondevices

| Feature                                            | Phase | Status |
|----------------------------------------------------|-------|--------|
| OAuth + MFA login, session persistence             | 1     | ✅     |
| Device listing                                     | 1     | ✅     |
| Speak / Announce / SetVolume                       | 1     | ✅     |
| Music, Sounds, Skills, Routines                    | 2     | ✅     |
| Sensors, DnD, Notifications, History               | 2     | ✅     |
| HTTP/2 push events                                 | 3     | ✅     |
| Smart-home: Power, Brightness, Color, ColorTemp    | 4     | ✅     |
| Smart-home: Thermostat, Lock                       | 4     | ✅     |
| Smart-home: Mode (typed) + Range/Toggle (stubbed)  | 4     | ✅¹    |

¹ `Device.Range` and `Device.Toggle` accessors are wired into the
capability-gate but their write methods return `ErrControllerNotImplemented`
pending a confirmed HAR capture of the wire payloads. File an issue with
a HAR if you have a device that exposes either controller.

## License

MIT. See [LICENSE](LICENSE).

This library is heavily inspired by
[aioamazondevices](https://github.com/chemelli74/aioamazondevices), which is
licensed under Apache-2.0. See [NOTICE](NOTICE) for attribution.
