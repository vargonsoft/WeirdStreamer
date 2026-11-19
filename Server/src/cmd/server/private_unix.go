//go:build unix

package main

import "syscall"

// privateUmask makes every file and folder the server (and ffmpeg, which it starts) creates
// readable by the server's account only.
func privateUmask() { syscall.Umask(0o077) }
