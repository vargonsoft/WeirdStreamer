//go:build !windows

package main

import (
	"fmt"
	"os"
)

// How to start and stop the server's service, for messages.
const (
	startHint = "sudo systemctl start weirdstreamer (or launchctl kickstart, or scripts/live.sh)"
	stopHint  = "sudo systemctl stop weirdstreamer"
)

// handleServiceCommand is Windows'; on Linux and macOS setup.sh installs the service.
func handleServiceCommand([]string) {
	fmt.Fprintln(os.Stderr, "cli service is for Windows; here, setup.sh service installs the server as a service (systemd or launchd).")
	os.Exit(2)
}
