// Package identify maps the files of a torrent to a show's episodes.
//
// Classify is pure: it reads file paths, sizes and a ShowCatalog, and
// returns one Proposal per file with its target episodes, a confidence and
// a short reason. It never imports the library or the subtitle packages;
// internal/service builds the catalog from a library show.
//
// The rules are built to know when they aren't sure: a high-confidence
// proposal may be applied without review, so the corpus test
// (testdata/corpus) holds them to zero wrong high results.
package identify

// TorrentFile is a file inside a torrent. Path runs from the torrent's root;
// in a multi-file torrent its first segment is the torrent's folder.
type TorrentFile struct {
	Path string
	Size int64
}

// Confidence is how sure a proposal is. High proposals may be applied
// without review.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
	// ConfidenceNone is a proposal without a target.
	ConfidenceNone Confidence = ""
)

// Rank orders confidences: none 0, low 1, medium 2, high 3.
func (c Confidence) Rank() int {
	switch c {
	case ConfidenceHigh:
		return 3
	case ConfidenceMedium:
		return 2
	case ConfidenceLow:
		return 1
	}
	return 0
}

// Valid reports whether c is high, medium or low.
func (c Confidence) Valid() bool { return c.Rank() > 0 }

// Kind is what a file is. The job contract (PLAN §6 "Assignment") folds it
// into kind, group and extra_type:
//   - episode: kind episode; group matched when high, else needs_review;
//   - special: kind episode; group specials when high, needs_review with a
//     lower confidence, unmatched without a target;
//   - unknown: kind episode, group unmatched;
//   - extra and sample: kind extra, group extras, with their ExtraType;
//   - subtitle: kind subtitle, following LinkedVideo.
type Kind string

const (
	KindEpisode  Kind = "episode"  // a regular episode (or several)
	KindSpecial  Kind = "special"  // a season-0 episode, or a file that looks like one
	KindExtra    Kind = "extra"    // openings, trailers, fonts, superseded versions...: ignored by default
	KindSample   Kind = "sample"   // a sample clip: ignored by default
	KindSubtitle Kind = "subtitle" // follows the video it belongs to
	KindUnknown  Kind = "unknown"  // a video the rules couldn't place
)

// ExtraType says what an extra (or a sample) is.
type ExtraType string

const (
	ExtraOpening    ExtraType = "opening"
	ExtraEnding     ExtraType = "ending"
	ExtraSample     ExtraType = "sample"
	ExtraTrailer    ExtraType = "trailer"
	ExtraFeaturette ExtraType = "featurette"
	ExtraFont       ExtraType = "font"
	ExtraInfo       ExtraType = "info"
	ExtraImage      ExtraType = "image"
	ExtraSuperseded ExtraType = "superseded"
	ExtraOther      ExtraType = "other"
)

// Source is who decided a proposal.
type Source string

const (
	SourceRules Source = "rules"
	SourceAI    Source = "ai"
)

// Method names the evidence behind a proposal's targets.
type Method string

const (
	MethodSxxEyy        Method = "sxxeyy"         // "S01E05" in the name
	MethodNxNN          Method = "nxnn"           // "1x05"
	MethodSeasonNumber  Method = "season_number"  // "S2 - 05", "Season 3 - 01"
	MethodFolderSeason  Method = "folder_season"  // a bare number read in the folder's (or the only) season
	MethodAbsolute      Method = "absolute"       // a bare number read as an absolute episode number
	MethodDate          Method = "date"           // an air date
	MethodRemapGroup    Method = "remap_group"    // a release season mapped through a TMDB episode group
	MethodRemapCour     Method = "remap_cour"     // a release season mapped to a cour of a TMDB season
	MethodRemapOverflow Method = "remap_overflow" // an episode past its season's end, counted on
	MethodSpecialNumber Method = "special_number" // "S00E05", "OVA 2"
	MethodTitle         Method = "title"          // the episode's name in the file name
	MethodConcatenated  Method = "concatenated"   // "101" read as S01E01
	MethodHint          Method = "hint"           // the user's hint
	MethodLinked        Method = "linked"         // a subtitle following its video
	MethodAI            Method = "ai"             // the AI resolver
)

// Target is an episode a file maps to.
type Target struct {
	EpisodeID int64 `json:"episode_id"`
	Season    int   `json:"season"`
	Episode   int   `json:"episode"`
}

// Proposal is the classifier's answer for one file.
type Proposal struct {
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	Kind      Kind      `json:"kind"`
	ExtraType ExtraType `json:"extra_type,omitempty"` // extras and samples
	// Targets are the episodes the file maps to, in season and episode
	// order: several for a multi-episode file, none when unmatched.
	Targets    []Target   `json:"targets"`
	Confidence Confidence `json:"confidence"` // ConfidenceNone without targets
	Method     Method     `json:"method,omitempty"`
	Source     Source     `json:"source"`
	Reason     string     `json:"reason"` // at most 120 characters
	// LinkedVideo is the video a subtitle belongs to.
	LinkedVideo string `json:"linked_video,omitempty"`
	// SupersededBy is the newer version (v2) that replaces this file.
	SupersededBy string `json:"superseded_by,omitempty"`
}

// HasTarget reports whether the proposal maps to at least one episode.
func (p *Proposal) HasTarget() bool { return len(p.Targets) > 0 }
