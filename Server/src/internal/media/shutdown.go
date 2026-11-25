package media

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// partPath is where a picture is made before it's renamed to path, so a stop part way never
// leaves a cut-off one under the real name: "frame-10.jpg" becomes "frame-10.part.jpg" (ffmpeg
// picks the format by the extension).
func partPath(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + ".part" + ext
}

// finishPart renames a finished picture made at partPath(path) to path.
func finishPart(path string) error {
	return os.Rename(partPath(path), path)
}

// shuttingDown is set once StopChildren runs, so the transcodes it stops don't log as failed.
var shuttingDown atomic.Bool
