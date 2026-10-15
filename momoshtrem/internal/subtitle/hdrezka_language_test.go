package subtitle

import "testing"

func TestHDRezkaLanguage(t *testing.T) {
	tests := []struct {
		code, name         string
		wantCode, wantName string
	}{
		{"en", "English", "en", "English"},
		{"ru", "Русский", "ru", "Russian"},
		// parseSubtitles maps the site's "ua"; one that slips through too.
		{"uk", "Українська", "uk", "Ukrainian"},
		{"ua", "Українська", "uk", "Ukrainian"},
		{" EN ", "English", "en", "English"},
		// No code: the name tells.
		{"", "Русский", "ru", "Russian"},
		{"", "Українська", "uk", "Ukrainian"},
		{"", "Английский", "en", "English"},
		{"", "English", "en", "English"},
		{"", "Ukrainian?", "uk", "Ukrainian"},
		// Neither: a slug of the name, which keeps the site's name.
		{"", "Қазақ тілі (субтитры)", "қазақ-тілі-субтитры", "Қазақ тілі (субтитры)"},
		{"", "Forced ../../etc", "forced-etc", "Forced ../../etc"},
		{"", "", "unknown", "Unknown"},
		{"", "  ", "unknown", "Unknown"},
		// A malformed code falls back to the name.
		{"../x", "English", "en", "English"},
		// A well-formed code the app doesn't name keeps the site's name.
		{"xx", "Klingon", "xx", "Klingon"},
		{"xx", "", "xx", "XX"},
	}
	for _, tt := range tests {
		t.Run(tt.code+"/"+tt.name, func(t *testing.T) {
			code, name := hdrezkaLanguage(tt.code, tt.name)
			if code != tt.wantCode || name != tt.wantName {
				t.Errorf("hdrezkaLanguage(%q, %q) = %q, %q; want %q, %q", tt.code, tt.name, code, name, tt.wantCode, tt.wantName)
			}
		})
	}
}

// A track HDRezka sends without a code, like Breaking Bad's "Русский
// (альтернативный)", is named after the language its name starts with.
func TestFileLanguage(t *testing.T) {
	alternative, _ := hdrezkaLanguage("", "Русский (альтернативный)")
	tests := []struct {
		source Source
		code   string
		want   string
	}{
		{SourceHDRezka, "en", "en"},
		{SourceHDRezka, alternative, "ru"},
		{SourceHDRezka, "қазақ-тілі-субтитры", "қазақ-тілі-субтитры"},
		{SourceHDRezka, "xx", "xx"},
		{SourceTorrent, "unknown", "unknown"},
		{SourceOpenSubtitles, "pt-br", "pt-br"},
	}
	for _, tt := range tests {
		sub := &Subtitle{Source: tt.source, LanguageCode: tt.code}
		if got := sub.FileLanguage(); got != tt.want {
			t.Errorf("%s %q: FileLanguage = %q, want %q", tt.source, tt.code, got, tt.want)
		}
	}
}
