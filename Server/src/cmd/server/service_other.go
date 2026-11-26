//go:build !windows

package main

import "context"

// runService is Windows' (cli service install); elsewhere systemd and launchd run the server
// as a program, so it's never a service here.
func runService(func(context.Context)) bool { return false }

// serviceLog is for a Windows service; elsewhere live.sh, systemd or launchd keep the log.
func serviceLog(string) {}
