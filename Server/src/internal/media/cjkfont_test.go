package media

import "testing"

func TestCJKCharacters(t *testing.T) {
	if got := CJKCharacters("Sign 12, English 12 — ASS en"); got != "" {
		t.Errorf("Latin text: got %q", got)
	}
	if got := CJKCharacters("日本語 の 字幕、中文 字幕，한국어 日本"); len([]rune(got)) != 13 {
		t.Errorf("CJK text: got %d distinct characters, want 13", len([]rune(got)))
	}
	got := CJKCharacters("日本 語 日")
	if len([]rune(got)) != 3 {
		t.Errorf("distinct characters: got %q", got)
	}
	for _, r := range "日本語の字幕、，한" {
		if !IsCJK(r) {
			t.Errorf("%q should be CJK", r)
		}
	}
	if IsCJK('é') || IsCJK('A') {
		t.Error("Latin letters aren't CJK")
	}
}
