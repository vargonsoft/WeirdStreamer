package media

import "testing"

func TestPreferredTracks(t *testing.T) {
	dub := &Codecs{
		Audio: []AudioCodec{{Index: 0, Language: "jpn", Default: true}, {Index: 1, Language: "eng"}},
		Subtitles: []SubtitleCodec{
			{Index: 0, Language: "eng", Forced: true, Text: true},
			{Index: 1, Language: "eng", Text: true, HearingImpaired: true},
			{Index: 2, Language: "eng", Text: true},
			{Index: 3, Language: "fre", Text: true},
		},
	}
	cases := []struct {
		name               string
		c                  *Codecs
		audioLang, subLang string
		on                 bool
		audio, sub         int
	}{
		{"file defaults", dub, "", "", false, -1, -1},
		{"dub in the language", dub, "en", "", false, 1, -1},
		{"audio in the language by its other code", &Codecs{Audio: []AudioCodec{{Index: 0, Language: "ger"}, {Index: 1, Language: "fre", Default: true}}}, "de", "", false, 0, -1},
		{"no audio in the language", dub, "de", "", false, -1, -1},
		{"full subtitles, not forced or SDH", dub, "", "en", true, -1, 2},
		{"subtitles whatever the audio", dub, "en", "en", true, 1, 2},
		{"other subtitles", dub, "ja", "fr", true, 0, 3},
		{"subtitles only for the hard of hearing", &Codecs{Audio: dub.Audio[:1], Subtitles: dub.Subtitles[:2]}, "", "en", true, -1, 1},
		{"no subtitles in the language", dub, "", "de", true, -1, -1},
		{"a language, subtitles off", dub, "", "fr", false, -1, -1},
		{"subtitles on, the file's default language", dub, "", "", true, -1, 2},
		{"DVD subtitles aren't drawn", &Codecs{Subtitles: []SubtitleCodec{{Index: 0, Language: "eng", Codec: "dvd_subtitle"}}}, "", "en", true, -1, -1},
		{"PGS are", &Codecs{Subtitles: []SubtitleCodec{{Index: 0, Language: "eng", Codec: "hdmv_pgs_subtitle"}}}, "", "en", true, -1, 0},
	}
	for _, c := range cases {
		a, s := PreferredTracks(c.c, c.audioLang, c.subLang, c.on)
		if a != c.audio || s != c.sub {
			t.Errorf("%s: got audio %d subtitle %d, want %d %d", c.name, a, s, c.audio, c.sub)
		}
	}
	if NormalizeLanguage("pt-BR") != "pt" || NormalizeLanguage("chi") != "zh" || NormalizeLanguage("xx") != "" {
		t.Error("NormalizeLanguage")
	}
}
