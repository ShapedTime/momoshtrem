// Package corpus loads the episode-mapping corpus: the cases in
// testdata/corpus that specify internal/identify, and the show fixtures in
// testdata/shows they run against. cmd/classifier-eval runs its golden
// cases against the AI resolver.
//
// It doesn't import identify, so identify's own tests can use it. Its Show
// mirrors identify.ShowCatalog: Episode and AltGroup convert directly
// (identify.Episode(e)), the rest field by field.
package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultNow is "today" for cases without their own.
var DefaultNow = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// Case is one torrent of the corpus. The schema is described at the top of
// testdata/corpus/anime.yaml.
type Case struct {
	ID           string   `yaml:"id"`
	Show         string   `yaml:"show"`
	Torrent      string   `yaml:"torrent"`
	Golden       bool     `yaml:"golden"`
	Tags         []string `yaml:"tags"`
	Now          string   `yaml:"now"`
	Hint         *Hint    `yaml:"hint"`
	DropSpecials bool     `yaml:"drop_specials"`
	Files        []File   `yaml:"files"`

	// Source is the corpus file the case is in, for messages.
	Source string `yaml:"-"`
}

// Hint is an assign hint.
type Hint struct {
	Season  *int `yaml:"season"`
	Episode *int `yaml:"episode"`
}

// File is one file of a case and what the classifier should make of it.
type File struct {
	Path      string `yaml:"path"`
	Size      string `yaml:"size"`
	Ambiguous bool   `yaml:"ambiguous"`
	Expect    Expect `yaml:"expect"`
}

// Expect is a file's expected answer.
type Expect struct {
	Kind       string    `yaml:"kind"`
	Eps        []string  `yaml:"eps"`
	Extra      string    `yaml:"extra"`
	MinConf    string    `yaml:"min_conf"`
	MaxConf    string    `yaml:"max_conf"`
	Acceptable []*string `yaml:"acceptable"`
}

// LoadCases reads every case in dir/corpus/*.yaml, where dir is the
// testdata directory.
func LoadCases(dir string) ([]Case, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "corpus", "*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no corpus files in %s", filepath.Join(dir, "corpus"))
	}
	var cases []Case
	ids := map[string]bool{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var cs []Case
		if err := yaml.Unmarshal(data, &cs); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		for i := range cs {
			if ids[cs[i].ID] {
				return nil, fmt.Errorf("%s: duplicate case %q", p, cs[i].ID)
			}
			ids[cs[i].ID] = true
			cs[i].Source = filepath.Base(p)
		}
		cases = append(cases, cs...)
	}
	return cases, nil
}

// NowTime is the case's "today": its own date, else DefaultNow.
func (c Case) NowTime() (time.Time, error) {
	if c.Now == "" {
		return DefaultNow, nil
	}
	now, err := time.Parse("2006-01-02", c.Now)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: now: %w", c.ID, err)
	}
	return now, nil
}

// TorrentName is the torrent's name; a single-file case defaults to the
// file's path.
func (c Case) TorrentName() (string, error) {
	if c.Torrent != "" {
		return c.Torrent, nil
	}
	if len(c.Files) != 1 {
		return "", fmt.Errorf("%s: a multi-file case needs a torrent name", c.ID)
	}
	return c.Files[0].Path, nil
}

// FilePath is the torrent path of file i: files of a multi-file case sit in
// a folder named after the torrent, as in a real torrent.
func (c Case) FilePath(i int) string {
	if len(c.Files) > 1 {
		return c.Torrent + "/" + c.Files[i].Path
	}
	return c.Files[i].Path
}

// ParseSize reads "1.2GB", "350MB", "40KB" or a byte count. Empty gives 0:
// the caller picks a typical size for the file's class.
func ParseSize(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	units := []struct {
		suffix string
		mult   float64
	}{{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSuffix(s, u.suffix), 64)
			return int64(n * u.mult), err
		}
	}
	return strconv.ParseInt(s, 10, 64)
}

// ExpectedKind is the kind (identify.Kind) an expectation implies.
func (e Expect) ExpectedKind() string {
	switch {
	case e.Kind != "":
		return e.Kind
	case e.Extra == "sample":
		return "sample"
	case e.Extra != "":
		return "extra"
	case len(e.Eps) > 0:
		for _, ep := range e.Eps {
			if !strings.HasPrefix(ep, "S00") {
				return "episode"
			}
		}
		return "special"
	}
	return "unknown"
}

// Label writes an episode like the corpus does: "S01E05".
func Label(season, episode int) string { return fmt.Sprintf("S%02dE%02d", season, episode) }

// JoinLabels writes a target set like the corpus does: "S01E01+S01E02"
// (sorted), or "null" for none.
func JoinLabels(labels []string) string {
	if len(labels) == 0 {
		return "null"
	}
	sorted := append([]string(nil), labels...)
	sort.Strings(sorted)
	return strings.Join(sorted, "+")
}

// Got is a classifier's answer for a file, in the corpus's terms.
type Got struct {
	Kind    string // identify.Kind
	Extra   string // identify.ExtraType
	Targets string // JoinLabels of the targets
}

// Correct reports whether got answers the file right. An ambiguous file is
// right when its targets are one of the acceptable answers; any other file
// needs its kind and targets, and its extra type when one is expected.
func (f File) Correct(got Got) bool {
	if f.Ambiguous {
		return f.Acceptable(got.Targets)
	}
	return got.Kind == f.Expect.ExpectedKind() && got.Targets == JoinLabels(f.Expect.Eps) &&
		(f.Expect.Extra == "" || got.Extra == f.Expect.Extra)
}

// Acceptable reports whether targets (a JoinLabels string) is one of an
// ambiguous file's acceptable answers.
func (f File) Acceptable(targets string) bool {
	for _, a := range f.Expect.Acceptable {
		want := "null"
		if a != nil {
			want = *a
		}
		if want == targets {
			return true
		}
	}
	return false
}

// AcceptableLabel lists an ambiguous file's acceptable answers.
func (f File) AcceptableLabel() string {
	parts := make([]string, len(f.Expect.Acceptable))
	for i, a := range f.Expect.Acceptable {
		parts[i] = "null"
		if a != nil {
			parts[i] = *a
		}
	}
	return strings.Join(parts, ", ")
}

// WantLabel describes an unambiguous file's expected answer: "episode
// S01E05", "extra null (opening)".
func (f File) WantLabel() string {
	s := f.Expect.ExpectedKind() + " " + JoinLabels(f.Expect.Eps)
	if f.Expect.Extra != "" {
		s += " (" + f.Expect.Extra + ")"
	}
	return s
}
