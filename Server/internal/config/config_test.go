package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadJSON(t *testing.T, body string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestLibraryType(t *testing.T) {
	cases := []struct {
		name    string
		typeVal string
		wantErr string
	}{
		{"none", ``, ""},
		{"movie", `, "type": "movie"`, ""},
		{"tv", `, "type": "tv"`, ""},
		{"unknown", `, "type": "music"`, `invalid type "music"`},
		{"case-sensitive", `, "type": "TV"`, `invalid type "TV"`},
		{"only one", `, "type": ["movie", "tv"]`, "cannot unmarshal"},
	}
	for _, c := range cases {
		err := loadJSON(t, `{"media": {"paths": [{"name": "Lib", "path": "/x", "groups": ["a"]`+c.typeVal+`}]}}`)
		if c.wantErr == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: error %v, want containing %q", c.name, err, c.wantErr)
		}
	}
}
