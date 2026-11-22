package media

import "strings"

// languageCodes are the languages an account can prefer, by their ISO 639-1 code, with the
// ISO 639-2 codes files tag tracks with (the terminology code, then the bibliographic one where
// it differs: "fra" and "fre").
var languageCodes = map[string][]string{
	"ar": {"ara"}, "bg": {"bul"}, "bn": {"ben"}, "ca": {"cat"}, "cs": {"ces", "cze"}, "da": {"dan"},
	"de": {"deu", "ger"}, "el": {"ell", "gre"}, "en": {"eng"}, "es": {"spa"}, "et": {"est"},
	"eu": {"eus", "baq"}, "fa": {"fas", "per"}, "fi": {"fin"}, "fil": {"fil"}, "fr": {"fra", "fre"},
	"ga": {"gle"}, "gl": {"glg"}, "he": {"heb"}, "hi": {"hin"}, "hr": {"hrv"}, "hu": {"hun"},
	"id": {"ind"}, "is": {"isl", "ice"}, "it": {"ita"}, "ja": {"jpn"}, "ko": {"kor"}, "lt": {"lit"},
	"lv": {"lav"}, "ms": {"msa", "may"}, "nl": {"nld", "dut"}, "no": {"nor", "nob", "nno"},
	"pl": {"pol"}, "pt": {"por"}, "ro": {"ron", "rum"}, "ru": {"rus"}, "sk": {"slk", "slo"},
	"sl": {"slv"}, "sr": {"srp"}, "sv": {"swe"}, "ta": {"tam"}, "te": {"tel"}, "th": {"tha"},
	"tr": {"tur"}, "uk": {"ukr"}, "ur": {"urd"}, "vi": {"vie"}, "zh": {"zho", "chi"},
}

// Languages lists the codes an account may choose (Settings names them in the reader's language).
func Languages() []string {
	out := make([]string, 0, len(languageCodes))
	for code := range languageCodes {
		out = append(out, code)
	}
	return out
}

// ValidLanguage reports whether code is one an account may choose ("" is none).
func ValidLanguage(code string) bool {
	_, ok := languageCodes[code]
	return ok || code == ""
}

// NormalizeLanguage turns a track's language tag ("eng", "fre", "en", "pt-BR") into the code
// an account chooses, or "" when it's unknown ("und", empty, or not one of Languages).
func NormalizeLanguage(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexAny(tag, "-_"); i > 0 {
		tag = tag[:i]
	}
	if _, ok := languageCodes[tag]; ok {
		return tag
	}
	for code, alts := range languageCodes {
		for _, a := range alts {
			if a == tag {
				return code
			}
		}
	}
	return ""
}

// PreferredTracks picks the tracks to play c with for an account that prefers audio in audioLang
// and subtitles in subtitleLang ("" for the file's default), with subtitles on or not, or -1 where
// players should do as the file says (its default audio; subtitles off but for forced ones).
//   - audio: a track in audioLang (the file's default among them, else the first);
//   - subtitles, when on: a full track (not forced) in subtitleLang, or in any language for the
//     file's default; the file's default among them, else one not for the hard of hearing, else the
//     first. Off, they start off as the file says.
func PreferredTracks(c *Codecs, audioLang, subtitleLang string, subtitlesOn bool) (audio, subtitle int) {
	audio, subtitle = -1, -1
	if c == nil {
		return
	}
	if audioLang != "" {
		for i, a := range c.Audio {
			if NormalizeLanguage(a.Language) == audioLang && (audio < 0 || a.Default && !c.Audio[audio].Default) {
				audio = i
			}
		}
		if audio >= 0 {
			audio = c.Audio[audio].Index
		}
	}
	if !subtitlesOn {
		return
	}
	rank := func(s SubtitleCodec) int {
		switch {
		case s.Default:
			return 2
		case !s.HearingImpaired:
			return 1
		}
		return 0
	}
	for i, s := range c.Subtitles {
		if s.Forced || subtitleLang != "" && NormalizeLanguage(s.Language) != subtitleLang || !s.Text && s.Codec != "hdmv_pgs_subtitle" {
			continue
		}
		if subtitle < 0 || rank(s) > rank(c.Subtitles[subtitle]) {
			subtitle = i
		}
	}
	if subtitle >= 0 {
		subtitle = c.Subtitles[subtitle].Index
	}
	return
}
