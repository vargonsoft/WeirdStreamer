package handlers

import (
	"encoding/json"
	"path/filepath"
)

// abs is a test's Unix-style disk path as this platform's absolute path ("/old/shows" is
// C:\old\shows on Windows).
func abs(p string) string {
	a, err := filepath.Abs(filepath.FromSlash(p))
	if err != nil {
		panic(err)
	}
	return a
}

// jsonString is s as a JSON string, quotes included (a Windows path's backslashes escaped).
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
