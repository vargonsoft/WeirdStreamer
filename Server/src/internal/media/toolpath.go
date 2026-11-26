package media

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// toolName is a program's file name: ffmpeg.exe on Windows.
func toolName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// UseBundledTools puts the tools beside the running binary first on the PATH: ffmpeg/bin next to
// its bin folder (a package's own ffmpeg, built by build-ffmpeg.sh, or Server/ffmpeg/bin in the
// repository). On macOS it also adds Homebrew's folders, which services (launchd) don't have on
// their PATH. ffmpeg, ffprobe and the other tools are run by name.
func UseBundledTools() {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			dir := filepath.Join(filepath.Dir(filepath.Dir(exe)), "ffmpeg", "bin")
			if _, err := os.Stat(filepath.Join(dir, toolName("ffmpeg"))); err == nil {
				dirs = append(dirs, dir)
			}
		}
	}
	path := os.Getenv("PATH")
	if runtime.GOOS == "darwin" {
		for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
			if !strings.Contains(":"+path+":", ":"+dir+":") {
				path += ":" + dir
			}
		}
	}
	if len(dirs) > 0 {
		sep := string(os.PathListSeparator)
		path = strings.Join(dirs, sep) + sep + path
	}
	os.Setenv("PATH", path)
}
