# goamazondevices

[![CI](https://github.com/Fishwaldo/goamazondevices/actions/workflows/ci.yml/badge.svg)](https://github.com/Fishwaldo/goamazondevices/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/Fishwaldo/goamazondevices.svg)](https://pkg.go.dev/github.com/Fishwaldo/goamazondevices)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

A Go library for controlling Amazon Echo / Alexa devices.

> **Status: pre-1.0 — port in progress.** Mirrors functionality of the Python library
> [aioamazondevices](https://github.com/chemelli74/aioamazondevices).
> See `CHANGELOG.md` for what's supported in the current release.

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

See `cmd/amazondevices-test/` for a full CLI example.

## CLI test harness

`cmd/amazondevices-test/` is a Cobra/Viper CLI mirroring the Python
`library_test.py`. Build and use:

```bash
go build -o amazondevices-test ./cmd/amazondevices-test

# Interactive login (prompts for OTP)
./amazondevices-test login --email you@example.com --password 'your-password'

# List devices
./amazondevices-test devices list

# Speak / announce / volume
./amazondevices-test speak    --serial S-KITCHEN-001 --text "hello"
./amazondevices-test announce --serial S-PAIR-001    --text "dinner ready"
./amazondevices-test volume   --serial S-KITCHEN-001 --value 30
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

| Feature | Phase | Status |
|---|---|---|
| OAuth + MFA login | 1 | ✅ |
| Session persistence | 1 | ✅ |
| Device listing | 1 | ✅ |
| Speak / Announce / SetVolume | 1 | ✅ |
| Music, Sounds, Skills, Routines | 2 | ⏳ |
| Sensors, DnD, Notifications, History | 2 | ⏳ |
| HTTP/2 push events | 3 | ⏳ |

## License

Apache 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
