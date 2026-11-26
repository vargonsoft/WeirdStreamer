package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// TypedChannel is a set of the Guide's channels an admin defines (docs/GUIDE.md, "Typed channels"):
// made only from its libraries, of its type, seen only by the groups that can open every one of them.
type TypedChannel struct {
	ID        string   `json:"id" yaml:"id"`               // made when it's added, kept
	Type      string   `json:"type" yaml:"type"`           // one of TypedChannelTypes
	Libraries []string `json:"libraries" yaml:"libraries"` // library IDs
	Max       int      `json:"max" yaml:"max"`             // up to this many channels (1 to TypedChannelMax), as the content allows
	Number    int      `json:"number" yaml:"number"`       // its block's first number: 100, 110… 190 (given when it's added)
}

// The types a typed channel can have: Late Night airs everything in its libraries, all day; the
// others take their libraries' TMDB titles of that genre.
var TypedChannelTypes = []string{"latenight", "kids", "cartoons", "comedy", "scifi", "drama", "reallife", "action", "horror"}

// Typed channels' limits and numbers
const (
	TypedChannelMax   = 4
	TypedNumberFirst  = 100
	TypedNumberLast   = 190 // the last block's first number
	TypedNumberStride = 10
)

// TypedChannelByID is the typed channel definition with the ID.
func (c *Config) TypedChannelByID(id string) (TypedChannel, bool) {
	for _, t := range c.Guide.TypedChannels {
		if t.ID == id {
			return t, true
		}
	}
	return TypedChannel{}, false
}

// InTypedChannel reports whether a typed channel uses the library (it's then in no regular channel).
func (c *Config) InTypedChannel(libraryID string) bool {
	for _, t := range c.Guide.TypedChannels {
		if slices.Contains(t.Libraries, libraryID) {
			return true
		}
	}
	return false
}

// TypedChannelGroups are the groups that see a typed channel: those that can open every one of its
// libraries (none when it has none).
func (c *Config) TypedChannelGroups(t TypedChannel) []string {
	var out []string
	for i, id := range t.Libraries {
		lib, ok := c.LibraryByID(id)
		if !ok {
			return nil
		}
		if i == 0 {
			out = slices.Clone(lib.Groups)
			continue
		}
		out = slices.DeleteFunc(out, func(g string) bool { return !slices.Contains(lib.Groups, g) })
	}
	return out
}

// validateTypedChannels checks the typed channels and fills in what's given when one is added: its
// ID, its count and its block of numbers. Libraries since removed are dropped from them.
func (c *Config) validateTypedChannels() error {
	if len(c.Guide.TypedChannels) > (TypedNumberLast-TypedNumberFirst)/TypedNumberStride+1 {
		return fmt.Errorf("guide.typedChannels: at most %d", (TypedNumberLast-TypedNumberFirst)/TypedNumberStride+1)
	}
	ids, numbers := map[string]bool{}, map[int]bool{}
	for i := range c.Guide.TypedChannels {
		t := &c.Guide.TypedChannels[i]
		t.Type = strings.ToLower(strings.TrimSpace(t.Type))
		if !slices.Contains(TypedChannelTypes, t.Type) {
			return fmt.Errorf("guide.typedChannels: invalid type %q (allowed: %s)", t.Type, strings.Join(TypedChannelTypes, ", "))
		}
		if t.ID == "" {
			b := make([]byte, 4)
			_, _ = rand.Read(b)
			t.ID = hex.EncodeToString(b)
		}
		if ids[t.ID] {
			return fmt.Errorf("guide.typedChannels: the id %q is used twice", t.ID)
		}
		ids[t.ID] = true
		var libs []string
		for _, id := range t.Libraries {
			if _, ok := c.LibraryByID(id); ok && !slices.Contains(libs, id) {
				libs = append(libs, id)
			}
		}
		t.Libraries = libs
		if t.Libraries == nil {
			t.Libraries = []string{}
		}
		if t.Max <= 0 || t.Max > TypedChannelMax {
			t.Max = TypedChannelMax
		}
		if t.Number < TypedNumberFirst || t.Number > TypedNumberLast || (t.Number-TypedNumberFirst)%TypedNumberStride != 0 || numbers[t.Number] {
			t.Number = 0 // given below, after the others keep theirs
		} else {
			numbers[t.Number] = true
		}
	}
	for i := range c.Guide.TypedChannels {
		t := &c.Guide.TypedChannels[i]
		for n := TypedNumberFirst; t.Number == 0; n += TypedNumberStride {
			if !numbers[n] {
				t.Number, numbers[n] = n, true
			}
		}
	}
	return nil
}
