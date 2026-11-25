package media

import (
	"runtime"
	"testing"
)

// skipOnWindows skips a test that can't run there, saying why.
func skipOnWindows(t *testing.T, why string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows: " + why)
	}
}
