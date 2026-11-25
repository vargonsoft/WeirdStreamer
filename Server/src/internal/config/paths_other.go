//go:build !windows

package config

// samePath: names are compared exactly (a case-insensitive macOS volume still has one name per file
// as the scanner lists it, and clientPartsOK leaves the rest to the file system).
func samePath(a, b string) bool { return a == b }

// clientPartsOK checks the parts of a client path below its library (rel, "/"-separated) before
// they're joined onto its folder (root). Off Windows any name is a file's own name; ".." that climbs
// out is caught after joining (DiskPathIn).
func clientPartsOK(root, rel string) bool { return true }
