// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

// Package alexactl is a Go client for Amazon Echo / Alexa devices.
//
// It provides functional parity with the Python aioamazondevices library
// (https://github.com/chemelli74/aioamazondevices) using idiomatic Go.
package alexactl

// Version is the build version of this library. It defaults to "dev" for
// builds made outside CI; release builds override it at link time via
//
//	-ldflags "-X github.com/Fishwaldo/alexactl.Version=<version>"
//
// set by the GitHub release workflow on tag push. Treat version.go as the
// single source of truth — read this var rather than hard-coding a version.
var Version = "dev"

// UpstreamVersion records the version of aioamazondevices whose behavior
// this library mirrors. Bumped when this library catches up to a new upstream tag.
const UpstreamVersion = "14.2.0"
