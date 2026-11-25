package database

import (
	"path/filepath"
	"strings"
)

// sep is the disk paths' separator, as stored by the scanner.
const sep = string(filepath.Separator)

// under is the range of disk paths inside the folder dir, for `path >= lo AND path < hi`: every
// path under dir starts with dir and its separator, and sorts before dir and the byte after the
// separator ('0' after '/', ']' after '\').
func under(dir string) (lo, hi string) {
	dir = strings.TrimSuffix(dir, sep)
	return dir + sep, dir + string(rune(filepath.Separator+1))
}
