# goamazondevices

[![CI](https://github.com/Fishwaldo/goamazondevices/actions/workflows/ci.yml/badge.svg)](https://github.com/Fishwaldo/goamazondevices/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/Fishwaldo/goamazondevices.svg)](https://pkg.go.dev/github.com/Fishwaldo/goamazondevices)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

A Go library for controlling Amazon Echo / Alexa devices and skill-discovered
smart-home endpoints (lights, switches, locks, thermostats, fans, …).

> **Status: v1.0 — API frozen.** Mirrors functionality of the Python library
> [aioamazondevices](https://github.com/chemelli74/aioamazondevices). The
> public surface follows semver: additive ports go in `MINOR`, signature
> changes in `MAJOR`. See `CHANGELOG.md` for per-release detail.

## Install

```bash
go get github.com/Fishwaldo/goamazondevices
```

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Fishwaldo/goamazondevices"
)

func main() {
    ctx := context.Background()

    client, err := amazondevices.NewClient()
    if err != nil {
        log.Fatal(err)
    }

    session, err := client.Auth.Login(ctx, "you@example.com", "your-password",
        amazondevices.OTPProviderFunc(func(ctx context.Context) (string, error) {
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
        _ = cc.Set(ctx, amazondevices.ColorFromRGB(255, 64, 0))
    }
    if lc := d.Lock(); lc != nil {
        _ = lc.Lock(ctx)
    }
    if tc := d.Thermostat(); tc != nil {
        _ = tc.SetTarget(ctx, 21, amazondevices.TemperatureScaleCelsius)
    }
    if mc := d.Mode("Apparatus.FanSpeed"); mc != nil {
        _ = mc.Set(ctx, "L3")
    }
}
```

See `cmd/amazondevices-test/` for a full CLI example.

## CLI test harness

`cmd/amazondevices-test/` is a Cobra/Viper CLI mirroring the Python
`library_test.py`. Build and use:

```bash
go build -o amazondevices-test ./cmd/amazondevices-test

# Interactive login (prompts for OTP)
./amazondevices-test login --email you@example.com --password 'your-password'

# Inventory + state
./amazondevices-test devices list
./amazondevices-test devices list --detail
./amazondevices-test sensors list

# Media (Echo speakers)
./amazondevices-test speak    --serial S-KITCHEN-001 --text "hello"
./amazondevices-test announce --serial S-PAIR-001    --text "dinner ready"
./amazondevices-test volume   --serial S-KITCHEN-001 --value 30

# Smart-home control
./amazondevices-test devices power      <serial> on
./amazondevices-test devices brightness <serial> 80
./amazondevices-test devices color      <serial> ff4000
./amazondevices-test devices lock       <serial>
./amazondevices-test devices thermostat <serial> set-target 21 c
./amazondevices-test devices mode set   <serial> Apparatus.FanSpeed L3
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

Apache 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
