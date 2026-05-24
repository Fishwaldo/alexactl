// Command amazondevices-test is the manual CLI smoke-test harness for
// goamazondevices. It mirrors the Python library_test.py: log in,
// optionally save the session, list devices, and exercise media commands.
package main

import (
	"os"

	"github.com/Fishwaldo/goamazondevices/cmd/amazondevices-test/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
