package database

import (
	"errors"
	"strings"
)

// Where a table keeps a media file or folder, it keeps it by its client path (config.ClientPath:
// the library's ID and the path below it, "01c6653f/Show/S01E01.mkv"), never by where it is on
// disk, so moving a library's folder changes no stored data: playback, segments,
// subtitle_offsets, dvd_discs, metadata_folders.folder_path and episode_overrides.file_path.
// Callers turn disk paths into client paths (config.ClientPath) before calling; a file outside
// every library has none, and nothing is stored for it.
//
// media_items and media_added are the exception: they are the scanner's record of what is on
// disk (it finds files by their disk paths, and every reader of a row opens the file), rebuilt by
// scans and dated by the files' own times, so they keep disk paths.

// ErrNotClientPath refuses a key that isn't a client path (empty, or a disk path).
var ErrNotClientPath = errors.New("not a client path")

// checkKey makes sure a disk path is never stored where a client path belongs.
func checkKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") {
		return ErrNotClientPath
	}
	return nil
}
