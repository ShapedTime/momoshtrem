package subtitle

import (
	"path/filepath"
	"regexp"
	"strings"
)

// languagePattern holds a compiled regex and the corresponding language info
type languagePattern struct {
	pattern *regexp.Regexp
	code    string
	name    string
}

// languagePatterns for detecting language from subtitle filenames
// Patterns match extensions like .en.srt, .eng.srt, .english.srt
var languagePatterns = []languagePattern{
	// ISO 639-1 (2-letter) and ISO 639-2 (3-letter) codes
	{regexp.MustCompile(`(?i)\.(?:en|eng)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "en", "English"},
	{regexp.MustCompile(`(?i)\.(?:ru|rus)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "ru", "Russian"},
	{regexp.MustCompile(`(?i)\.(?:tr|tur)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "tr", "Turkish"},
	{regexp.MustCompile(`(?i)\.(?:az|aze)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "az", "Azerbaijani"},
	{regexp.MustCompile(`(?i)\.(?:es|spa)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "es", "Spanish"},
	{regexp.MustCompile(`(?i)\.(?:de|deu|ger)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "de", "German"},
	{regexp.MustCompile(`(?i)\.(?:fr|fra|fre)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "fr", "French"},
	{regexp.MustCompile(`(?i)\.(?:it|ita)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "it", "Italian"},
	{regexp.MustCompile(`(?i)\.(?:pt|por)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "pt", "Portuguese"},
	{regexp.MustCompile(`(?i)\.(?:ja|jpn)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "ja", "Japanese"},
	{regexp.MustCompile(`(?i)\.(?:ko|kor)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "ko", "Korean"},
	{regexp.MustCompile(`(?i)\.(?:zh|chi|chs|cht|zho)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "zh", "Chinese"},
	{regexp.MustCompile(`(?i)\.(?:ar|ara)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "ar", "Arabic"},
	{regexp.MustCompile(`(?i)\.(?:hi|hin)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "hi", "Hindi"},
	{regexp.MustCompile(`(?i)\.(?:pl|pol)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "pl", "Polish"},
	{regexp.MustCompile(`(?i)\.(?:nl|dut|nld)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "nl", "Dutch"},
	{regexp.MustCompile(`(?i)\.(?:sv|swe)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "sv", "Swedish"},
	{regexp.MustCompile(`(?i)\.(?:no|nor)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "no", "Norwegian"},
	{regexp.MustCompile(`(?i)\.(?:da|dan)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "da", "Danish"},
	{regexp.MustCompile(`(?i)\.(?:fi|fin)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "fi", "Finnish"},
	{regexp.MustCompile(`(?i)\.(?:cs|cze|ces)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "cs", "Czech"},
	{regexp.MustCompile(`(?i)\.(?:hu|hun)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "hu", "Hungarian"},
	{regexp.MustCompile(`(?i)\.(?:ro|ron|rum)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "ro", "Romanian"},
	{regexp.MustCompile(`(?i)\.(?:el|gre|ell)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "el", "Greek"},
	{regexp.MustCompile(`(?i)\.(?:he|heb)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "he", "Hebrew"},
	{regexp.MustCompile(`(?i)\.(?:th|tha)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "th", "Thai"},
	{regexp.MustCompile(`(?i)\.(?:vi|vie)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "vi", "Vietnamese"},
	{regexp.MustCompile(`(?i)\.(?:id|ind)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "id", "Indonesian"},
	{regexp.MustCompile(`(?i)\.(?:ms|msa|may)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "ms", "Malay"},
	{regexp.MustCompile(`(?i)\.(?:uk|ukr)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "uk", "Ukrainian"},
	{regexp.MustCompile(`(?i)\.(?:bg|bul)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "bg", "Bulgarian"},
	{regexp.MustCompile(`(?i)\.(?:hr|hrv)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "hr", "Croatian"},
	{regexp.MustCompile(`(?i)\.(?:sr|srp)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "sr", "Serbian"},
	{regexp.MustCompile(`(?i)\.(?:sk|slk|slo)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "sk", "Slovak"},
	{regexp.MustCompile(`(?i)\.(?:sl|slv)\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "sl", "Slovenian"},

	// Full language names
	{regexp.MustCompile(`(?i)\.english\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "en", "English"},
	{regexp.MustCompile(`(?i)\.russian\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "ru", "Russian"},
	{regexp.MustCompile(`(?i)\.turkish\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "tr", "Turkish"},
	{regexp.MustCompile(`(?i)\.spanish\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "es", "Spanish"},
	{regexp.MustCompile(`(?i)\.german\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "de", "German"},
	{regexp.MustCompile(`(?i)\.french\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "fr", "French"},
	{regexp.MustCompile(`(?i)\.italian\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "it", "Italian"},
	{regexp.MustCompile(`(?i)\.portuguese\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "pt", "Portuguese"},
	{regexp.MustCompile(`(?i)\.japanese\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "ja", "Japanese"},
	{regexp.MustCompile(`(?i)\.korean\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "ko", "Korean"},
	{regexp.MustCompile(`(?i)\.chinese\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "zh", "Chinese"},
	{regexp.MustCompile(`(?i)\.arabic\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "ar", "Arabic"},
	{regexp.MustCompile(`(?i)\.hindi\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "hi", "Hindi"},
	{regexp.MustCompile(`(?i)\.polish\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "pl", "Polish"},
	{regexp.MustCompile(`(?i)\.dutch\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "nl", "Dutch"},
	{regexp.MustCompile(`(?i)\.swedish\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "sv", "Swedish"},
	{regexp.MustCompile(`(?i)\.norwegian\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "no", "Norwegian"},
	{regexp.MustCompile(`(?i)\.danish\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "da", "Danish"},
	{regexp.MustCompile(`(?i)\.finnish\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "fi", "Finnish"},
	{regexp.MustCompile(`(?i)\.czech\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "cs", "Czech"},
	{regexp.MustCompile(`(?i)\.hungarian\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "hu", "Hungarian"},
	{regexp.MustCompile(`(?i)\.romanian\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "ro", "Romanian"},
	{regexp.MustCompile(`(?i)\.greek\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "el", "Greek"},
	{regexp.MustCompile(`(?i)\.hebrew\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "he", "Hebrew"},
	{regexp.MustCompile(`(?i)\.thai\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "th", "Thai"},
	{regexp.MustCompile(`(?i)\.vietnamese\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "vi", "Vietnamese"},
	{regexp.MustCompile(`(?i)\.indonesian\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "id", "Indonesian"},
	{regexp.MustCompile(`(?i)\.ukrainian\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "uk", "Ukrainian"},
	{regexp.MustCompile(`(?i)\.bulgarian\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "bg", "Bulgarian"},
	{regexp.MustCompile(`(?i)\.croatian\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "hr", "Croatian"},
	{regexp.MustCompile(`(?i)\.serbian\.(?:srt|sub|ass|ssa|vtt|idx|smi)$`), "sr", "Serbian"},
}

// directoryLanguages maps directory names to language info
var directoryLanguages = map[string]struct{ code, name string }{
	"english":    {"en", "English"},
	"eng":        {"en", "English"},
	"russian":    {"ru", "Russian"},
	"rus":        {"ru", "Russian"},
	"turkish":    {"tr", "Turkish"},
	"tur":        {"tr", "Turkish"},
	"azerbaijani": {"az", "Azerbaijani"},
	"spanish":    {"es", "Spanish"},
	"german":     {"de", "German"},
	"french":     {"fr", "French"},
	"italian":    {"it", "Italian"},
	"portuguese": {"pt", "Portuguese"},
	"japanese":   {"ja", "Japanese"},
	"korean":     {"ko", "Korean"},
	"chinese":    {"zh", "Chinese"},
	"arabic":     {"ar", "Arabic"},
	"hindi":      {"hi", "Hindi"},
	"polish":     {"pl", "Polish"},
	"dutch":      {"nl", "Dutch"},
	"swedish":    {"sv", "Swedish"},
	"norwegian":  {"no", "Norwegian"},
	"danish":     {"da", "Danish"},
	"finnish":    {"fi", "Finnish"},
	"czech":      {"cs", "Czech"},
	"hungarian":  {"hu", "Hungarian"},
	"romanian":   {"ro", "Romanian"},
	"greek":      {"el", "Greek"},
	"hebrew":     {"he", "Hebrew"},
	"thai":       {"th", "Thai"},
	"vietnamese": {"vi", "Vietnamese"},
	"indonesian": {"id", "Indonesian"},
	"ukrainian":  {"uk", "Ukrainian"},
	"bulgarian":  {"bg", "Bulgarian"},
	"croatian":   {"hr", "Croatian"},
	"serbian":    {"sr", "Serbian"},
	"subs":       {"", ""},  // Common folder name, but no specific language
}

// DetectLanguage attempts to detect the language from a subtitle filename.
// Returns (languageCode, languageName, detected).
// If no language is detected, returns ("unknown", "Unknown", false).
func DetectLanguage(filename string) (code string, name string, detected bool) {
	// First, try to match filename patterns (e.g., .en.srt, .english.srt)
	for _, lp := range languagePatterns {
		if lp.pattern.MatchString(filename) {
			return lp.code, lp.name, true
		}
	}

	// Fallback: check parent directory names
	// e.g., "Subs/English/subtitle.srt" or "eng/subtitle.srt"
	dir := filepath.Dir(filename)
	parts := strings.Split(dir, string(filepath.Separator))

	for _, part := range parts {
		partLower := strings.ToLower(part)
		if lang, ok := directoryLanguages[partLower]; ok && lang.code != "" {
			return lang.code, lang.name, true
		}
	}

	return "unknown", "Unknown", false
}
