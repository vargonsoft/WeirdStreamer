package subtitles

import "strings"

// languages: ISO 639-2/B codes (as ffprobe reports a video's own tracks) with the two-letter
// ISO 639-1 code OpenSubtitles uses, other three-letter forms and the English name.
var languages = []struct{ code, short, alt, name string }{
	{"eng", "en", "", "english"},
	{"spa", "es", "", "spanish"},
	{"fre", "fr", "fra", "french"},
	{"ger", "de", "deu", "german"},
	{"ita", "it", "", "italian"},
	{"por", "pt", "", "portuguese"},
	{"dut", "nl", "nld", "dutch"},
	{"swe", "sv", "", "swedish"},
	{"nor", "no", "nob", "norwegian"},
	{"dan", "da", "", "danish"},
	{"fin", "fi", "", "finnish"},
	{"ice", "is", "isl", "icelandic"},
	{"pol", "pl", "", "polish"},
	{"cze", "cs", "ces", "czech"},
	{"slo", "sk", "slk", "slovak"},
	{"hun", "hu", "", "hungarian"},
	{"rum", "ro", "ron", "romanian"},
	{"bul", "bg", "", "bulgarian"},
	{"hrv", "hr", "", "croatian"},
	{"srp", "sr", "", "serbian"},
	{"slv", "sl", "", "slovenian"},
	{"gre", "el", "ell", "greek"},
	{"tur", "tr", "", "turkish"},
	{"rus", "ru", "", "russian"},
	{"ukr", "uk", "", "ukrainian"},
	{"heb", "he", "", "hebrew"},
	{"ara", "ar", "", "arabic"},
	{"per", "fa", "fas", "persian"},
	{"hin", "hi", "", "hindi"},
	{"tha", "th", "", "thai"},
	{"vie", "vi", "", "vietnamese"},
	{"ind", "id", "", "indonesian"},
	{"may", "ms", "msa", "malay"},
	{"chi", "zh", "zho", "chinese"},
	{"jpn", "ja", "", "japanese"},
	{"kor", "ko", "", "korean"},
	{"est", "et", "", "estonian"},
	{"lav", "lv", "", "latvian"},
	{"lit", "lt", "", "lithuanian"},
	{"cat", "ca", "", "catalan"},
	{"baq", "eu", "eus", "basque"},
	{"glg", "gl", "", "galician"},
}

// Language reads a language from a file name tag or an OpenSubtitles code (en, eng, english,
// pt-br, zh-cn…) as an ISO 639-2/B code; "" if it isn't one. "hi" counts as hearing impaired in
// file names (sidecar), so it's only Hindi when it comes from OpenSubtitles.
func Language(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexAny(tag, "-_"); i > 0 {
		tag = tag[:i] // pt-br, zh-cn, zh-tw
	}
	switch tag {
	case "pob", "pb":
		return "por" // OpenSubtitles' Brazilian Portuguese
	}
	for _, l := range languages {
		if tag == l.code || tag == l.short || (l.alt != "" && tag == l.alt) || tag == l.name {
			return l.code
		}
	}
	return ""
}

// Short is the two-letter code OpenSubtitles searches by, from any form Language reads.
func Short(tag string) string {
	code := Language(tag)
	for _, l := range languages {
		if l.code == code {
			return l.short
		}
	}
	return ""
}
