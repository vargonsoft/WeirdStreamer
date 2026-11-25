package config

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// samePath: Windows' file systems ignore case.
func samePath(a, b string) bool { return strings.EqualFold(a, b) }

// clientPartsOK checks the parts of a client path below its library (rel, "/"-separated) before
// they're joined onto its folder (root). A client path names a file by the names the scanner
// listed, so on Windows a part is refused when it would reach a file by another name, or reach
// something other than a file in that folder:
//   - "\" (another separator), ":" (a drive, or a file's alternate data stream), wildcards and the
//     other characters Windows doesn't allow in names, and ".." (clients never climb);
//   - a trailing dot or space (Windows drops them: "Private." is "Private");
//   - device names (CON, NUL, COM1…, with any extension);
//   - a name that exists under another spelling: another case, or an 8.3 short name ("PRIVAT~1"),
//     so one file has one client path and a library inside another can't be reached through it.
func clientPartsOK(root, rel string) bool {
	dir := root
	exists := true
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." {
			continue
		}
		if part == ".." || !validName(part) {
			return false
		}
		dir = filepath.Join(dir, part)
		if !exists {
			continue // nothing below a missing folder can name an existing file
		}
		name, ok := storedName(dir)
		if !ok {
			exists = false
			continue
		}
		if name != part {
			return false
		}
	}
	return true
}

// validName is false for a name Windows would read as something else, or doesn't allow.
func validName(name string) bool {
	if strings.ContainsAny(name, `\/:*?"<>|`) || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return false
	}
	for _, r := range name {
		if r < 32 {
			return false
		}
	}
	base := strings.ToUpper(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " "))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return false
	}
	if len(base) >= 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) {
		switch base[3:] {
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³":
			return false
		}
	}
	return true
}

// storedName is the name the file or folder at path has in its folder (its long name, in its own
// case), or false when there's none.
func storedName(path string) (string, bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", false
	}
	var data windows.Win32finddata
	h, err := windows.FindFirstFile(p, &data)
	if err != nil {
		return "", false
	}
	windows.FindClose(h)
	return windows.UTF16ToString(data.FileName[:]), true
}
