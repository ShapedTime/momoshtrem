package subtitle

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// HDRezkaTrack is one of an HDRezka stream's subtitle tracks, downloaded.
type HDRezkaTrack struct {
	// Code is the site's language code: ISO 639-1 ("uk" for its "ua"),
	// "" when it gives none.
	Code string
	// Name is the site's name for the language: "English", "Русский".
	Name string
	VTT  []byte
}

// HDRezkaStream is the stream an item's tracks came with.
type HDRezkaStream struct {
	// File is the item's assignment file_path (library.StreamRef.FilePath):
	// the subtitles are stored only while it's the item's active file.
	File string
	// Release is the stream's label, "HDRezka · Оригинал (+субтитры) ·
	// 1080p": the subtitles' release name.
	Release string
}

// SaveHDRezka converts an item's HDRezka tracks to SRT and stores them as
// its HDRezka subtitles, in {download_path}/hdrezka/{item_type}/{item_id}/
// {lang}.srt: one per language, each replacing the item's HDRezka
// subtitle in that language, so assigning the stream again refreshes them.
// A track that isn't WebVTT, or whose language an earlier track took, is
// skipped and logged. Files of languages the item no longer has are
// removed. Nothing is stored, and ErrStaleStream returned, when the stream
// is no longer the item's file.
func (s *Service) SaveHDRezka(ctx context.Context, item Item, stream HDRezkaStream, tracks []HDRezkaTrack) ([]*Subtitle, error) {
	dir := filepath.Join(s.subtitlesPath, string(SourceHDRezka), string(item.Type), strconv.FormatInt(item.ID, 10))
	log := slog.With("item_type", item.Type, "item_id", item.ID)

	type pendingFile struct{ tmp, final string }
	var subs []*Subtitle
	var files []pendingFile
	defer func() {
		for _, f := range files {
			if f.tmp != "" {
				os.Remove(f.tmp)
			}
		}
	}()
	taken := map[string]bool{}
	for _, track := range tracks {
		code, name := hdrezkaLanguage(track.Code, track.Name)
		if taken[code] {
			log.Warn("Skipping an HDRezka subtitle track in a language already stored", "language", code, "name", track.Name)
			continue
		}
		srt, err := VTTToSRT(track.VTT)
		if err != nil {
			log.Warn("Skipping an HDRezka subtitle track that isn't usable WebVTT", "language", code, "error", err)
			continue
		}
		if len(files) == 0 {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("failed to create subtitle directory: %w", err)
			}
		}
		tmp, err := writeTemp(dir, srt)
		if err != nil {
			return nil, fmt.Errorf("failed to save subtitle file: %w", err)
		}
		taken[code] = true
		final := filepath.Join(dir, code+".srt")
		files = append(files, pendingFile{tmp: tmp, final: final})
		subs = append(subs, &Subtitle{
			ItemType:     item.Type,
			ItemID:       item.ID,
			LanguageCode: code,
			LanguageName: name,
			Format:       "srt",
			FilePath:     final,
			FileSize:     int64(len(srt)),
			ReleaseName:  stream.Release,
		})
	}
	if len(subs) == 0 {
		return nil, nil
	}

	// The files move into place inside the transaction that holds the
	// stream, so an assign that replaces it can't interleave.
	err := s.repo.UpsertHDRezka(ctx, item, stream.File, subs, func() error {
		for i := range files {
			if err := os.Rename(files[i].tmp, files[i].final); err != nil {
				return fmt.Errorf("failed to save subtitle file: %w", err)
			}
			files[i].tmp = ""
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.removeStaleHDRezkaFiles(ctx, item, dir)

	langs := make([]string, len(subs))
	for i, sub := range subs {
		langs[i] = sub.LanguageCode
	}
	log.Info("HDRezka subtitles stored", "languages", strings.Join(langs, ","), "release", stream.Release)
	return subs, nil
}

// removeStaleHDRezkaFiles removes the SRT files in the item's HDRezka
// directory that none of its HDRezka subtitles names: languages an
// earlier stream had and this one hasn't. Their rows went when the file
// changed.
func (s *Service) removeStaleHDRezkaFiles(ctx context.Context, item Item, dir string) {
	subs, err := s.repo.GetByItem(ctx, item.Type, item.ID)
	if err != nil {
		slog.Warn("Failed to list subtitles to tidy the HDRezka directory", "item_id", item.ID, "error", err)
		return
	}
	keep := map[string]bool{}
	for _, sub := range subs {
		if sub.Source == SourceHDRezka {
			keep[sub.FilePath] = true
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() || filepath.Ext(e.Name()) != ".srt" || keep[path] {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			slog.Warn("Failed to remove a stale HDRezka subtitle file", "path", path, "error", err)
		}
	}
}

// hdrezkaLanguages maps the names HDRezka gives subtitle tracks (its own,
// mostly Russian, and English ones), as slugs, to language codes, for a
// track the site sends without a code.
var hdrezkaLanguages = func() map[string]string {
	m := map[string]string{
		"русский": "ru", "русские": "ru", "английский": "en", "английские": "en",
		"українська": "uk", "украинский": "uk", "украинские": "uk",
		"беларуская": "be", "белорусский": "be", "қазақша": "kk", "казахский": "kk",
		"немецкий": "de", "deutsch": "de", "французский": "fr", "français": "fr",
		"испанский": "es", "español": "es", "итальянский": "it", "italiano": "it",
		"турецкий": "tr", "türkçe": "tr", "азербайджанский": "az", "azərbaycan": "az",
		"японский": "ja", "корейский": "ko", "китайский": "zh", "польский": "pl", "polski": "pl",
	}
	for name, lang := range directoryLanguages {
		if lang.code != "" {
			m[name] = lang.code
		}
	}
	return m
}()

// hdrezkaLanguage is the code and English name a track is stored under:
// the site's code when it's well formed, else the code its name maps to,
// else a slug of the name, so the one-per-language index holds and the
// file name stays safe ("unknown" when there's neither). A known code gets
// its English name; another keeps the site's.
func hdrezkaLanguage(code, name string) (string, string) {
	name = strings.TrimSpace(name)
	c, ok := NormalizeLanguageCode(code)
	if c == "ua" {
		c = "uk" // HDRezka's code for Ukrainian
	}
	if !ok {
		c = slug(name)
		if known, found := hdrezkaLanguages[c]; found {
			c = known
		}
	}
	if c == "" {
		c = "unknown"
	}
	if english, known := languageNames[c]; known {
		return c, english
	}
	if name == "" {
		return c, LanguageName(c)
	}
	return c, name
}

// slug lowercases s and keeps its letters and digits, the rest becoming
// single dashes: "Português (BR)" is "português-br". It never holds a dot
// or a path separator.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	return b.String()
}
