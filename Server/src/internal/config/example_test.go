package config

import (
	"os"
	"path/filepath"
	"testing"
)

// The example loads, and a config written from it with its own values is the example (the
// ids Load gives the libraries aside).
func TestExampleConfig(t *testing.T) {
	example := filepath.Join("..", "..", "..", "config.example.yml")
	if err := Load(example); err != nil {
		t.Fatal(err)
	}
	c := *GlobalConfig
	c.Media.Paths = append([]MediaPath(nil), c.Media.Paths...)
	for i := range c.Media.Paths {
		c.Media.Paths[i].ID = ""
	}
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := SaveFrom(example, path, &c); err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(example)
	got, _ := os.ReadFile(path)
	if string(got) != string(want) {
		t.Errorf("written from the example:\n%s", got)
	}
}
