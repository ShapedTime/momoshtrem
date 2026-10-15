package ai

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"golang.org/x/text/unicode/norm"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/identify/token"
)

const (
	// filesPerCall bounds one request's <resolve> list so its answer fits
	// max_tokens without streaming. Bigger scopes are split into several
	// requests that share (and cache) the context.
	filesPerCall = 60
	// collapseOver is the number of videos above which <files> lists only
	// the files in scope and their neighbours, and collapses the runs
	// between.
	collapseOver = 400
	// windowOver is the catalog size above which <episodes> lists only the
	// episodes near the numbers in the file names, plus every special.
	windowOver = 300
	// window is how far, in episodes, the window reaches around a number.
	window = 10
	// dateWindow is how far, in days, it reaches around an air date.
	dateWindow = 7
	// coursGap splits a season into parts: a longer break between two
	// episodes starts a new part (the classifier's cour rule).
	coursGap = 56 * 24 * time.Hour
)

// request is one Messages call: the context shared by every request for a
// torrent, and this call's files to resolve.
type request struct {
	context string
	ask     string
	// paths are the files in ask (full torrent paths), in order.
	paths []string
	// shared marks a context that other requests reuse: it gets a cache
	// breakpoint of its own.
	shared bool
	// folder is the torrent's top folder, stripped from listed paths.
	folder string
	// episodes is how many episodes the context lists.
	episodes int
	// noRules: the caller passed no rules' answers, so none are shown.
	noRules bool
}

// maxTokens sizes the answer to the files asked about: room for low-effort
// thinking plus about 150 tokens per file, under the non-streaming limit.
func (q request) maxTokens() int64 {
	return int64(min(4000+150*len(q.paths), 16000))
}

// params is the Messages request: the stable system prompt first, cached
// across torrents, then the torrent's context and the files to answer for.
func (q request) params(model string) anthropic.MessageNewParams {
	ctxBlock := anthropic.TextBlockParam{Text: q.context}
	if q.shared {
		ctxBlock.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	return anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: q.maxTokens(),
		System: []anthropic.TextBlockParam{{
			Text:         systemPrompt,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(
			anthropic.ContentBlockParamUnion{OfText: &ctxBlock},
			anthropic.NewTextBlock(q.ask),
		)},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortLow,
			Format: anthropic.JSONOutputFormatParam{Schema: outputSchema},
		},
	}
}

// answerFile is one answer as the schema has it: an AIFile and its number
// in <resolve>.
type answerFile struct {
	N int `json:"n"`
	identify.AIFile
}

// parse reads the answer. It checks the stop reason before the content (a
// refusal or a cut-off answer may not match the schema), maps each answer
// to its file by n, with the path as a check (or, when n doesn't fit, by
// the path alone, Unicode-normalized), and drops answers for files not
// asked about.
func (q request) parse(resp *anthropic.Message) ([]identify.AIFile, error) {
	switch resp.StopReason {
	case anthropic.StopReasonRefusal:
		if c := string(resp.StopDetails.Category); c != "" {
			return nil, fmt.Errorf("%w (%s)", ErrRefused, c)
		}
		return nil, ErrRefused
	case anthropic.StopReasonMaxTokens, anthropic.StopReasonModelContextWindowExceeded:
		return nil, fmt.Errorf("%w (%s)", ErrTruncated, resp.StopReason)
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(tb.Text)
		}
	}
	var answer struct {
		Files []answerFile `json:"files"`
	}
	if err := json.Unmarshal([]byte(text.String()), &answer); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadAnswer, err)
	}

	// A path names a file by its full path, its listed path or (when no
	// other file asked about has it) its file name, compared in NFC: macOS
	// writes NFD, models tend to answer NFC.
	byName := make(map[string]string, 3*len(q.paths))
	baseCount := map[string]int{}
	for _, p := range q.paths {
		baseCount[nfc(path.Base(p))]++
	}
	for _, p := range q.paths {
		byName[nfc(p)], byName[nfc(q.listed(p))] = p, p
		if base := nfc(path.Base(p)); baseCount[base] == 1 {
			byName[base] = p
		}
	}
	names := func(answer, full string) bool {
		a := nfc(answer)
		return a == nfc(full) || a == nfc(q.listed(full)) || a == nfc(path.Base(full))
	}

	seen := map[string]bool{}
	out := make([]identify.AIFile, 0, len(answer.Files))
	for _, a := range answer.Files {
		full := ""
		if a.N >= 1 && a.N <= len(q.paths) && (strings.TrimSpace(a.Path) == "" || names(a.Path, q.paths[a.N-1])) {
			full = q.paths[a.N-1]
		} else {
			full = byName[nfc(a.Path)] // n is off, or names another file: the path decides
		}
		if full == "" || seen[full] {
			continue
		}
		seen[full] = true
		f := a.AIFile
		f.Path = full
		out = append(out, f)
	}
	return out, nil
}

// nfc trims and NFC-normalizes a path for comparing.
func nfc(s string) string { return norm.NFC.String(strings.TrimSpace(s)) }

// listed is a path as the prompt writes it, without the torrent's folder.
func (q request) listed(p string) string {
	if q.folder != "" {
		return strings.TrimPrefix(p, q.folder+"/")
	}
	return p
}

// buildRequests writes the requests for a torrent. The output depends only
// on the input (sorted, no maps iterated in order), so equal inputs give
// byte-equal requests and the cache prefix holds.
func buildRequests(in identify.ResolveInput, scope []string) []request {
	cat := in.Show
	if cat == nil {
		cat = &identify.ShowCatalog{}
	}
	files := filesOf(in)
	base := request{folder: commonFolder(files), noRules: len(in.Rules) == 0}

	sortedScope := slices.Clone(scope)
	slices.SortStableFunc(sortedScope, naturalCompare)
	inScope := make(map[string]bool, len(scope))
	for _, p := range scope {
		inScope[p] = true
	}

	var b strings.Builder
	writeTorrent(&b, in.TorrentName, base.folder, files)
	writeShow(&b, cat)
	writeSeasons(&b, cat)
	writeAltOrderings(&b, cat)
	base.episodes = writeEpisodes(&b, cat, files, sortedScope)
	writeFiles(&b, base, files, inScope)
	if hint := strings.TrimSpace(in.Hint); hint != "" {
		fmt.Fprintf(&b, "<hint>\n%s\n</hint>\n", hint)
	}
	context := b.String()

	byPath := make(map[string]identify.Proposal, len(files))
	for _, p := range files {
		byPath[p.Path] = p
	}
	// Requests of one wave run side by side, so none reads what another
	// writes: the shared context is only worth caching for later waves.
	chunks := (len(sortedScope) + filesPerCall - 1) / filesPerCall
	var reqs []request
	for start := 0; start < len(sortedScope); start += filesPerCall {
		chunk := sortedScope[start:min(start+filesPerCall, len(sortedScope))]
		q := base
		q.context, q.paths = context, chunk
		q.shared = chunks > maxConcurrentCalls
		q.ask = writeResolve(q, byPath, start/filesPerCall+1, chunks)
		reqs = append(reqs, q)
	}
	return reqs
}

func writeTorrent(b *strings.Builder, name, folder string, files []identify.Proposal) {
	videos := 0
	for _, p := range files {
		if identify.ClassOf(p.Path) == identify.ClassVideo {
			videos++
		}
	}
	b.WriteString("<torrent>\n")
	fmt.Fprintf(b, "name: %s\n", name)
	if folder != "" {
		fmt.Fprintf(b, "folder: %s (paths below start inside it)\n", folder)
	}
	fmt.Fprintf(b, "files: %d, of which %d videos\n", len(files), videos)
	b.WriteString("</torrent>\n")
}

func writeShow(b *strings.Builder, cat *identify.ShowCatalog) {
	b.WriteString("<show>\n")
	fmt.Fprintf(b, "title: %s\n", cat.Title)
	if cat.OriginalTitle != "" && cat.OriginalTitle != cat.Title {
		fmt.Fprintf(b, "original title: %s\n", cat.OriginalTitle)
	}
	if len(cat.Aliases) > 0 {
		fmt.Fprintf(b, "other titles: %s\n", strings.Join(cat.Aliases, "; "))
	}
	if cat.Year > 0 {
		fmt.Fprintf(b, "first aired: %d\n", cat.Year)
	}
	if len(cat.Countries) > 0 {
		fmt.Fprintf(b, "origin: %s\n", strings.Join(cat.Countries, ", "))
	}
	anime := "no"
	if cat.IsAnime {
		anime = "yes"
	}
	fmt.Fprintf(b, "anime: %s\n", anime)
	switch cat.Ordering {
	case identify.OrderingAbsolute:
		b.WriteString("episode ordering: absolute (the library expects releases to count episodes by absolute number)\n")
	case identify.OrderingEpisodeGroup:
		b.WriteString("episode ordering: an alternative ordering (see <alternative_orderings>)\n")
	default:
		b.WriteString("episode ordering: as aired (TMDB's seasons)\n")
	}
	b.WriteString("</show>\n")
}

// seasonsOf groups the catalog's episodes by season, in order.
func seasonsOf(cat *identify.ShowCatalog) (numbers []int, bySeason map[int][]identify.Episode) {
	bySeason = map[int][]identify.Episode{}
	for _, ep := range cat.Episodes {
		bySeason[ep.Season] = append(bySeason[ep.Season], ep)
	}
	for n, eps := range bySeason {
		numbers = append(numbers, n)
		sort.Slice(eps, func(i, j int) bool { return eps[i].Number < eps[j].Number })
	}
	sort.Ints(numbers)
	return numbers, bySeason
}

func writeSeasons(b *strings.Builder, cat *identify.ShowCatalog) {
	numbers, bySeason := seasonsOf(cat)
	b.WriteString("<seasons>\n")
	if len(numbers) == 0 {
		b.WriteString("TMDB lists no episodes for this show.\n")
	}
	for _, n := range numbers {
		eps := bySeason[n]
		if n == 0 {
			fmt.Fprintf(b, "S00 (specials): %d episodes\n", len(eps))
			continue
		}
		fmt.Fprintf(b, "S%02d: %d episodes", n, len(eps))
		lo, hi := 0, 0
		for _, ep := range eps {
			if ep.Absolute > 0 {
				if lo == 0 || ep.Absolute < lo {
					lo = ep.Absolute
				}
				hi = max(hi, ep.Absolute)
			}
		}
		if lo > 0 {
			fmt.Fprintf(b, ", #%d-#%d", lo, hi)
		}
		parts := partsOf(eps)
		switch {
		case len(parts) == 1:
			fmt.Fprintf(b, ", aired %s", parts[0].dates())
		case len(parts) > 1:
			fmt.Fprintf(b, ", aired in %d parts:", len(parts))
			for i, p := range parts {
				sep := ";"
				if i == 0 {
					sep = ""
				}
				fmt.Fprintf(b, "%s E%02d-E%02d %s", sep, p.first, p.last, p.dates())
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("</seasons>\n")
}

// part is a run of a season's episodes without a long break.
type part struct {
	first, last  int
	start, end   string
	startT, endT time.Time
}

func (p part) dates() string {
	if p.start == p.end {
		return p.start
	}
	return p.start + " to " + p.end
}

// partsOf splits a season's dated episodes at breaks over coursGap.
func partsOf(eps []identify.Episode) []part {
	var parts []part
	for _, ep := range eps {
		t, err := time.Parse("2006-01-02", ep.AirDate)
		if err != nil {
			continue
		}
		if n := len(parts); n > 0 && t.Sub(parts[n-1].endT) <= coursGap {
			parts[n-1].last, parts[n-1].end, parts[n-1].endT = ep.Number, ep.AirDate, t
			continue
		}
		parts = append(parts, part{first: ep.Number, last: ep.Number, start: ep.AirDate, end: ep.AirDate, startT: t, endT: t})
	}
	return parts
}

func writeAltOrderings(b *strings.Builder, cat *identify.ShowCatalog) {
	if len(cat.AltOrderings) == 0 {
		return
	}
	byID := make(map[int64]identify.Episode, len(cat.Episodes))
	for _, ep := range cat.Episodes {
		byID[ep.ID] = ep
	}
	b.WriteString("<alternative_orderings>\n")
	for _, ao := range cat.AltOrderings {
		fmt.Fprintf(b, "%s:", ao.Name)
		for i, g := range ao.Groups {
			sep := ";"
			if i == 0 {
				sep = ""
			}
			fmt.Fprintf(b, "%s season %d = %s", sep, g.Number, episodeRuns(g.EpisodeIDs, byID))
		}
		b.WriteString("\n")
	}
	b.WriteString("</alternative_orderings>\n")
}

// episodeRuns writes episode IDs as runs: "S01E14-S01E26, S00E02".
func episodeRuns(ids []int64, byID map[int64]identify.Episode) string {
	var runs []string
	var first, last *identify.Episode
	flush := func() {
		if first == nil {
			return
		}
		run := label(first.Season, first.Number)
		if last.Number != first.Number {
			run += "-" + label(last.Season, last.Number)
		}
		runs = append(runs, run)
	}
	for _, id := range ids {
		ep, ok := byID[id]
		if !ok {
			continue
		}
		if last != nil && ep.Season == last.Season && ep.Number == last.Number+1 {
			e := ep
			last = &e
			continue
		}
		flush()
		e := ep
		first, last = &e, &e
	}
	flush()
	if len(runs) == 0 {
		return "(no episodes TMDB lists)"
	}
	return strings.Join(runs, ", ")
}

// writeEpisodes lists the catalog's episodes, a window of them for a long
// show, and returns how many it listed.
func writeEpisodes(b *strings.Builder, cat *identify.ShowCatalog, files []identify.Proposal, scope []string) int {
	eps := slices.Clone(cat.Episodes)
	sort.SliceStable(eps, func(i, j int) bool {
		if eps[i].Season != eps[j].Season {
			return eps[i].Season < eps[j].Season
		}
		return eps[i].Number < eps[j].Number
	})
	total := len(eps)
	if total > windowOver {
		eps = episodeWindow(cat, eps, files, scope)
		fmt.Fprintf(b, "<episodes listed=\"%d of %d\" note=\"only the episodes near the numbers in the file names, and every special\">\n", len(eps), total)
	} else {
		b.WriteString("<episodes>\n")
	}
	for _, ep := range eps {
		b.WriteString(label(ep.Season, ep.Number))
		if ep.Absolute > 0 && ep.Season > 0 {
			fmt.Fprintf(b, " #%d", ep.Absolute)
		}
		if ep.AirDate != "" {
			b.WriteString(" " + ep.AirDate)
		} else {
			b.WriteString(" (no date)")
		}
		if t := episodeType(ep.Type); t != "" {
			b.WriteString(" [" + t + "]")
		}
		if ep.Name != "" {
			b.WriteString(" | " + ep.Name)
		}
		b.WriteString("\n")
	}
	b.WriteString("</episodes>\n")
	return len(eps)
}

func episodeType(t string) string {
	switch t {
	case "", "standard":
		return ""
	case "mid_season":
		return "mid-season finale"
	}
	return strings.ReplaceAll(t, "_", " ")
}

// wordRe finds the words compared between title-only file names and
// episode names: four letters or more. digitsRe finds a number inside a
// word.
var (
	wordRe   = regexp.MustCompile(`[\pL]{4,}`)
	digitsRe = regexp.MustCompile(`[0-9]{1,4}`)
)

// stopWords are common words too weak to tie a file to an episode name.
var stopWords = map[string]bool{
	"with": true, "from": true, "that": true, "this": true, "what": true, "when": true, "will": true,
	"your": true, "into": true, "part": true, "episode": true, "special": true, "season": true,
	"complete": true, "final": true, "english": true, "subs": true, "dubbed": true,
}

// maxTitleMatches caps the episodes title words may add: a word common to
// that many names doesn't point at any of them.
const maxTitleMatches = 40

// episodeWindow picks a long show's episodes worth listing: those near the
// rules' targets and the markers in the files' names (episode and absolute
// numbers, ranges, SxxEyy, air dates, read by identify/token, which
// already sets resolutions, codecs, CRCs and years aside), those whose name
// shares enough words with a file named only by its title, and every
// special.
func episodeWindow(cat *identify.ShowCatalog, eps []identify.Episode, files []identify.Proposal, scope []string) []identify.Episode {
	type se struct{ season, episode int }
	ses := map[se]bool{}
	abs := map[int]bool{}
	var spans [][2]int
	var dates []time.Time
	var titled [][]string // the words of each file named only by its title

	showWords := map[string]bool{}
	for _, name := range append([]string{cat.Title, cat.OriginalTitle}, cat.Aliases...) {
		for _, w := range wordRe.FindAllString(strings.ToLower(name), -1) {
			showWords[w] = true
		}
	}

	for _, p := range files {
		for _, t := range p.Targets {
			ses[se{t.Season, t.Episode}] = true
		}
	}
	for _, p := range scope {
		found := false
		var seasons, numbers []int
		var words, wordNumbers []string
		segments := strings.Split(p, "/")
		for i, seg := range segments {
			if i == len(segments)-1 {
				seg = strings.TrimSuffix(seg, path.Ext(seg))
			}
			for _, t := range token.Tokenize(seg) {
				switch t.Kind {
				case token.SxxEyy, token.NxNN:
					for _, e := range t.Eps {
						ses[se{t.Season, e}] = true
					}
					found = true
				case token.Season:
					for s := t.Season; s <= t.LastSeason; s++ {
						seasons = append(seasons, s)
					}
				case token.Episode:
					numbers = append(numbers, t.Eps...)
					found = true
				case token.Num:
					if t.N > 0 {
						numbers = append(numbers, t.N)
						found = true
					}
				case token.Range:
					spans = append(spans, [2]int{t.Lo, t.Hi})
					found = true
				case token.Date:
					for _, d := range t.Dates {
						if day, err := time.Parse("2006-01-02", d); err == nil {
							dates = append(dates, day)
							found = true
						}
					}
				case token.Word:
					if i == len(segments)-1 {
						for _, w := range wordRe.FindAllString(t.Norm, -1) {
							if !showWords[w] && !stopWords[w] && !slices.Contains(words, w) {
								words = append(words, w)
							}
						}
						wordNumbers = append(wordNumbers, digitsRe.FindAllString(t.Norm, -1)...)
					}
				}
			}
		}
		// A name with no marker may still hide its number inside a word
		// ("第1071話").
		if !found && len(numbers) == 0 {
			for _, d := range wordNumbers {
				if n, err := strconv.Atoi(d); err == nil && n > 0 {
					numbers = append(numbers, n)
					found = true
				}
			}
		}
		for _, n := range numbers {
			abs[n] = true
			for _, s := range seasons {
				ses[se{s, n}] = true
			}
		}
		if !found && len(words) > 0 {
			titled = append(titled, words)
		}
	}

	// An S/E past a season's end may be counted on: window its absolute
	// number too.
	absStart := map[int]int{}
	for _, ep := range eps {
		if ep.Season > 0 && ep.Absolute > 0 && (absStart[ep.Season] == 0 || ep.Absolute < absStart[ep.Season]) {
			absStart[ep.Season] = ep.Absolute
		}
	}
	for k := range ses {
		if start, ok := absStart[k.season]; ok {
			abs[start+k.episode-1] = true
		}
	}

	nearAbs := func(n int) bool {
		for d := -window; d <= window; d++ {
			if abs[n+d] {
				return true
			}
		}
		for _, s := range spans {
			if n >= s[0]-window && n <= s[1]+window {
				return true
			}
		}
		return false
	}
	keep := make([]bool, len(eps))
	for i, ep := range eps {
		k := ep.Season == 0 || (ep.Absolute > 0 && nearAbs(ep.Absolute))
		for d := -window; !k && d <= window; d++ {
			k = ses[se{ep.Season, ep.Number + d}]
		}
		if !k && ep.AirDate != "" && len(dates) > 0 {
			if t, err := time.Parse("2006-01-02", ep.AirDate); err == nil {
				for _, d := range dates {
					if diff := t.Sub(d); diff <= dateWindow*24*time.Hour && diff >= -dateWindow*24*time.Hour {
						k = true
					}
				}
			}
		}
		keep[i] = k
	}

	// A title-only file names an episode that shares two of its words (its
	// one word, when it has one), not counting the show's own words.
	if len(titled) > 0 {
		var byTitle []int
		for i, ep := range eps {
			if keep[i] {
				continue
			}
			name := map[string]bool{}
			for _, w := range wordRe.FindAllString(strings.ToLower(ep.Name), -1) {
				name[w] = true
			}
			for _, words := range titled {
				shared := 0
				for _, w := range words {
					if name[w] {
						shared++
					}
				}
				if shared >= min(2, len(words)) {
					byTitle = append(byTitle, i)
					break
				}
			}
		}
		if len(byTitle) <= maxTitleMatches {
			for _, i := range byTitle {
				keep[i] = true
			}
		}
	}

	var out []identify.Episode
	for i, ep := range eps {
		if keep[i] {
			out = append(out, ep)
		}
	}
	return out
}

func writeFiles(b *strings.Builder, q request, files []identify.Proposal, inScope map[string]bool) {
	var videos []identify.Proposal
	others := map[identify.FileClass]int{}
	for _, p := range files {
		if c := identify.ClassOf(p.Path); c == identify.ClassVideo {
			videos = append(videos, p)
		} else {
			others[c]++
		}
	}
	slices.SortStableFunc(videos, func(a, b identify.Proposal) int { return naturalCompare(a.Path, b.Path) })

	keep := make([]bool, len(videos))
	for i, p := range videos {
		if len(videos) <= collapseOver || inScope[p.Path] {
			keep[i] = true
			if i > 0 {
				keep[i-1] = true
			}
			if i+1 < len(videos) {
				keep[i+1] = true
			}
		}
	}

	b.WriteString("<files>\n")
	for i := 0; i < len(videos); {
		if keep[i] {
			p := videos[i]
			fmt.Fprintf(b, "%s | %s", humanSize(p.Size), q.listed(p.Path))
			if !q.noRules {
				b.WriteString(" | rules: " + rulesLabel(p))
			}
			b.WriteString("\n")
			i++
			continue
		}
		j := i
		for j < len(videos) && !keep[j] {
			j++
		}
		run := videos[i:j]
		fmt.Fprintf(b, "[%d more files, %s to %s", len(run), q.listed(run[0].Path), q.listed(run[len(run)-1].Path))
		if !q.noRules {
			b.WriteString("; rules: " + summarize(run))
		}
		b.WriteString("]\n")
		i = j
	}
	if len(videos) == 0 {
		b.WriteString("(no video files)\n")
	}
	if line := otherFiles(others); line != "" {
		b.WriteString(line + "\n")
	}
	b.WriteString("</files>\n")
}

// otherFiles counts the files <files> doesn't list.
func otherFiles(counts map[identify.FileClass]int) string {
	names := []struct {
		class        identify.FileClass
		one, several string
	}{
		{identify.ClassSubtitle, "subtitle", "subtitles"},
		{identify.ClassFont, "font", "fonts"},
		{identify.ClassImage, "image", "images"},
		{identify.ClassInfo, "info file", "info files"},
		{identify.ClassArchive, "archive", "archives"},
		{identify.ClassOther, "other file", "other files"},
	}
	var parts []string
	total := 0
	for _, n := range names {
		c := counts[n.class]
		total += c
		switch {
		case c == 1:
			parts = append(parts, "1 "+n.one)
		case c > 1:
			parts = append(parts, fmt.Sprintf("%d %s", c, n.several))
		}
	}
	if total == 0 {
		return ""
	}
	return fmt.Sprintf("(also %s, not listed)", strings.Join(parts, ", "))
}

func writeResolve(q request, byPath map[string]identify.Proposal, part, parts int) string {
	var b strings.Builder
	b.WriteString("<resolve>\n")
	if parts > 1 {
		fmt.Fprintf(&b, "Part %d of %d of the files to answer for. ", part, parts)
	}
	fmt.Fprintf(&b, "Answer for these %d files, in this order:\n", len(q.paths))
	for i, p := range q.paths {
		r := byPath[p]
		fmt.Fprintf(&b, "%d. %s\n   size %s", i+1, q.listed(p), humanSize(r.Size))
		if !q.noRules {
			b.WriteString("; rules: " + rulesLabel(r))
			if r.Reason != "" {
				b.WriteString("; why: " + r.Reason)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("</resolve>")
	return b.String()
}

// rulesLabel is the rules' answer in a few words: "S01E05 high", "extra
// (opening)", "no match".
func rulesLabel(p identify.Proposal) string {
	switch {
	case p.Kind == identify.KindExtra:
		if p.ExtraType != "" {
			return "extra (" + string(p.ExtraType) + ")"
		}
		return "extra"
	case p.Kind == identify.KindSample:
		return "sample"
	case p.HasTarget():
		labels := make([]string, len(p.Targets))
		for i, t := range p.Targets {
			labels[i] = label(t.Season, t.Episode)
		}
		return strings.Join(labels, "+") + " " + string(p.Confidence)
	}
	return "no match"
}

// summarize counts the rules' answers in a collapsed run.
func summarize(run []identify.Proposal) string {
	counts := map[string]int{}
	for _, p := range run {
		switch {
		case p.Kind == identify.KindExtra || p.Kind == identify.KindSample:
			counts["extras"]++
		case p.HasTarget():
			counts[string(p.Confidence)]++
		default:
			counts["no match"]++
		}
	}
	var parts []string
	for _, k := range []string{"high", "medium", "low", "no match", "extras"} {
		if counts[k] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
		}
	}
	return strings.Join(parts, ", ")
}

// commonFolder is the first path segment every file shares, if any: a
// multi-file torrent's folder.
func commonFolder(files []identify.Proposal) string {
	folder := ""
	for i, p := range files {
		first, _, ok := strings.Cut(p.Path, "/")
		if !ok || (i > 0 && first != folder) {
			return ""
		}
		folder = first
	}
	return folder
}

func label(season, episode int) string { return fmt.Sprintf("S%02dE%02d", season, episode) }

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// naturalCompare orders paths with their numbers compared as numbers, so
// "Episode 9" comes before "Episode 10". Paths that only differ in leading
// zeros fall back to byte order, so the order never depends on the input's.
func naturalCompare(a, b string) int {
	if c := naturalCmp(a, b); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

func naturalCmp(a, b string) int {
	for a != "" && b != "" {
		da, db := digitPrefix(a), digitPrefix(b)
		if da != "" && db != "" {
			na, _ := strconv.ParseUint(da, 10, 64)
			nb, _ := strconv.ParseUint(db, 10, 64)
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		if a[0] != b[0] {
			if a[0] < b[0] {
				return -1
			}
			return 1
		}
		a, b = a[1:], b[1:]
	}
	return strings.Compare(a, b)
}

func digitPrefix(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}
