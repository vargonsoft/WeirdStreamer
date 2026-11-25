//go:build !windows

package handlers

// topIsDrives: the top is the file system's root, "/".
const topIsDrives = false

const absoluteExample = "starting with /"

func drives() []folderEntry { return nil }

// hiddenFolder: hidden folders are the ones named ".name", already left out.
func hiddenFolder(path string) bool { return false }
