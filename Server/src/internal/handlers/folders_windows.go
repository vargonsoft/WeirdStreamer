package handlers

import (
	"golang.org/x/sys/windows"
)

// topIsDrives: Windows has no one root: the top lists its drives.
const topIsDrives = true

const absoluteExample = `starting with its drive, as C:\Videos (or a share, as \\server\videos)`

// drives are the drives with a letter that answer (an empty card reader doesn't).
func drives() []folderEntry {
	out := []folderEntry{}
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return out
	}
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, _ := windows.UTF16PtrFromString(root)
		if _, err := windows.GetFileAttributes(p); err != nil {
			continue
		}
		out = append(out, folderEntry{Name: root, Path: root})
	}
	return out
}

// hiddenFolder is true for a folder Windows hides (Hidden or System: $Recycle.Bin, System
// Volume Information).
func hiddenFolder(path string) bool {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := windows.GetFileAttributes(p)
	return err == nil && attrs&(windows.FILE_ATTRIBUTE_HIDDEN|windows.FILE_ATTRIBUTE_SYSTEM) != 0
}
