package identify

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/identify/token"
)

// Evidence weights. A reading's weight says how much it alone would be
// trusted; agreement and contradiction adjust it (decide).
const (
	wExplicit      = 0.95 // "S01E05", "1x05", a unique air date
	wSeasonNumber  = 0.9  // "S2 - 05", "Season 3 - 01"
	wTitle         = 0.9  // the episode's name, unique, similarity ≥ 0.9
	wAbsolute      = 0.85 // a bare number read as absolute, for anime
	wFolderSeason  = 0.8  // a bare number in the folder's (or the only) season
	wSpecialNumber = 0.75 // "S00E05": TMDB numbers specials its own way
	wBare          = 0.7  // a bare number with weaker context
	wSpecialNamed  = 0.7  // "OVA 2" as the 2nd special named OVA
	wConcatSeason  = 0.7  // "101" as S01E01 in a "Season 1" folder
	wSpecialMarker = 0.6  // "OVA 3" as S00E03
	wNearDate      = 0.6  // an air date a day off, or shared by several episodes
	wPartial       = 0.6  // some of a multi-episode file's episodes exist
	wGuess         = 0.5  // a reading the context argues against
	wConcat        = 0.55 // "101" as S01E01

	bonusAgree    = 0.1  // a second, independent reading agrees
	penaltyContra = 0.3  // a strong reading disagrees
	contraMin     = 0.7  // how strong a disagreeing reading must be
	agreeMin      = 0.6  // how strong an agreeing reading must be
	highMin       = 0.85 // high: at least this...
	highMargin    = 0.2  // ...and this far ahead of any other answer
	mediumMin     = 0.6
	titleMin      = 0.9 // name similarity a title reading needs
)

// option is one reading of a file: the episodes it points at and how much
// the evidence weighs.
type option struct {
	eps    []*Episode
	weight float64
	method Method
	reason string
}

func (o option) key() string {
	ids := make([]string, len(o.eps))
	for i, ep := range o.eps {
		ids[i] = strconv.FormatInt(ep.ID, 10)
	}
	return strings.Join(ids, ",")
}

func (o option) label() string { return seLabel(o.eps) }

// options lists every reading of a video's candidates.
func (st *packState) options(fi *fileInfo) []option {
	var opts []option
	for _, c := range fi.cands {
		switch c.kind {
		case candExplicit:
			opts = append(opts, st.explicitOptions(fi, c)...)
		case candNumber:
			opts = append(opts, st.numberOptions(fi, c)...)
		case candDate:
			opts = append(opts, st.dateOptions(c)...)
		case candSpecial:
			opts = append(opts, st.specialOptions(c)...)
		case candConcat:
			if ep := st.ix.episode(c.season, c.eps[0]); ep != nil {
				w := wConcat
				if st.seasonFor(fi) == c.season {
					w = wConcatSeason
				}
				opts = append(opts, option{eps: []*Episode{ep}, weight: w, method: MethodConcatenated,
					reason: fmt.Sprintf("%q read as %s", c.text, seLabel([]*Episode{ep}))})
			}
		}
	}
	if o, ok := st.titleOption(fi); ok {
		opts = append(opts, o)
	}
	if len(opts) == 0 && len(fi.cands) == 0 {
		opts = append(opts, st.folderOptions(fi)...)
	}
	if len(opts) == 0 && st.hintEp > 0 && st.videos == 1 {
		season := st.hintSeason
		if season < 0 {
			season = 1
		}
		if ep := st.ix.episode(season, st.hintEp); ep != nil {
			opts = append(opts, option{eps: []*Episode{ep}, weight: wFolderSeason, method: MethodHint,
				reason: "From the hint: " + seLabel([]*Episode{ep})})
		}
	}
	return opts
}

// folderOptions reads the episode from the folder of a file whose own name
// says nothing ("The.Boys.S04E06.1080p.WEB/2f8a9c41e.mkv"), when it's the
// folder's only video.
func (st *packState) folderOptions(fi *fileInfo) []option {
	if len(fi.dirs) == 0 || st.videosIn[fi.dir] != 1 {
		return nil
	}
	for _, t := range token.Tokenize(fi.dirs[len(fi.dirs)-1]) {
		if (t.Kind != token.SxxEyy && t.Kind != token.NxNN) || t.Season < 1 {
			continue
		}
		if eps := st.ix.lookupAll(t.Season, nonZero(t.Eps)); len(eps) > 0 {
			return []option{{eps: eps, weight: wSeasonNumber, method: MethodSxxEyy,
				reason: fmt.Sprintf("%q in the folder name", t.Text)}}
		}
		return nil
	}
	return nil
}

func (st *packState) explicitOptions(fi *fileInfo, c candidate) []option {
	ix := st.ix
	if c.season == 0 {
		if eps := ix.lookupAll(0, c.eps); eps != nil {
			return []option{{eps: eps, weight: wSpecialNumber, method: MethodSpecialNumber,
				reason: fmt.Sprintf("%q in the name", c.text)}}
		}
		return nil
	}
	w := wExplicit
	if c.method == MethodSeasonNumber {
		w = wSeasonNumber
	}
	if eps := ix.lookupAll(c.season, c.eps); eps != nil {
		return []option{{eps: eps, weight: w, method: c.method, reason: fmt.Sprintf("%q in the name", c.text)}}
	}
	var some []*Episode
	var missing []int
	for _, e := range c.eps {
		if ep := ix.episode(c.season, e); ep != nil {
			some = append(some, ep)
		} else {
			missing = append(missing, e)
		}
	}
	if len(some) > 0 {
		return []option{{eps: some, weight: wPartial, method: c.method,
			reason: fmt.Sprintf("%q in the name; TMDB has no E%02d", c.text, missing[0])}}
	}
	if p := st.plan(fi.dir, c.season); p != nil {
		if eps := p.lookupAll(c.eps); eps != nil {
			return []option{{eps: eps, weight: p.weight(), method: p.method,
				reason: fmt.Sprintf("Release %s → %s (%s)", seText(c.season, c.eps), seLabel(eps), p.label)}}
		}
	}
	return nil
}

// numberOptions reads a bare episode number in the folder's (or the only)
// season, as an absolute number, and through a remap when the folder names
// a season TMDB doesn't have. The pack decides which reading weighs most.
func (st *packState) numberOptions(fi *fileInfo, c candidate) []option {
	ix := st.ix
	if fi.region.match == regionPrefixExtra {
		fi.suffixWhy = fmt.Sprintf("Name says %q; the season is unclear", strings.Join(fi.region.words, " "))
		return nil
	}
	s := st.seasonFor(fi)
	fits := st.fits(fi, s)
	absPref := ix.absPreferred() || st.NameRange != [2]int{}
	n := numText(c)
	var opts []option

	switch {
	case s == 0:
		if eps := ix.lookupAll(0, c.eps); eps != nil {
			opts = append(opts, option{eps: eps, weight: wBare, method: MethodSpecialNumber,
				reason: fmt.Sprintf("Specials folder + episode %s", n)})
		}
	case s >= 1 && ix.seasonExists(s):
		w := wFolderSeason
		if !fits {
			w = mediumMin
		}
		if eps := ix.lookupAll(s, c.eps); eps != nil {
			opts = append(opts, option{eps: eps, weight: w, method: MethodFolderSeason,
				reason: fmt.Sprintf("%s + episode %s", st.seasonSource(fi), n)})
		}
	case s >= 1:
		if p := st.plan(fi.dir, s); p != nil {
			if eps := p.lookupAll(c.eps); eps != nil {
				opts = append(opts, option{eps: eps, weight: p.weight(), method: p.method,
					reason: fmt.Sprintf("Release %s → %s (%s)", seText(s, c.eps), seLabel(eps), p.label)})
			}
		}
	case len(ix.regular) == 1:
		if eps := ix.lookupAll(ix.regular[0], c.eps); eps != nil {
			opts = append(opts, option{eps: eps, weight: wFolderSeason, method: MethodFolderSeason,
				reason: fmt.Sprintf("Episode %s of the show's only season", n)})
		}
	default:
		if len(ix.regular) > 0 {
			if eps := ix.lookupAll(ix.regular[0], c.eps); eps != nil {
				opts = append(opts, option{eps: eps, weight: wGuess, method: MethodFolderSeason,
					reason: fmt.Sprintf("Episode %s; no season in the name, read as S%02d", n, ix.regular[0])})
			}
		}
	}

	if eps := ix.lookupAbs(c.eps); eps != nil && s != 0 {
		w := wBare - 0.1
		switch {
		case s >= 1 && fits:
			w = wGuess
		case s >= 1 && !ix.seasonExists(s) && len(opts) > 0:
			w = wGuess // the remap explains the numbers
		case absPref:
			w = wAbsolute
		case s < 0 && len(ix.regular) == 1:
			w = wBare
		}
		if s >= 1 && !fits && allInSeason(eps, s) {
			w += bonusAgree // "Season 2/13.mkv" where S02 starts at #13
		}
		opts = append(opts, option{eps: eps, weight: w, method: MethodAbsolute,
			reason: fmt.Sprintf("Absolute #%s → %s", n, seLabel(eps))})
		if s < 0 && st.NameRange == [2]int{} {
			if o, ok := st.airingSeasonOption(eps, c, n); ok {
				opts = append(opts, o)
			}
		}
	}
	return opts
}

// airingSeasonDays is how recently a season must have started for a bare
// small number to be read as one of its episodes.
const airingSeasonDays = 180

// airingSeasonOption is the doubt about a bare small number while a later
// season airs: "[Group] Show - 05" may be the new season's E05 from a group
// that numbers each season from 01, not absolute #5 from years ago. It
// weighs enough to keep the absolute reading from being high.
func (st *packState) airingSeasonOption(abs []*Episode, c candidate, n string) (option, bool) {
	ix := st.ix
	latest := ix.lastRegular()
	if len(ix.regular) < 2 || allInSeason(abs, latest) {
		return option{}, false
	}
	first := ix.episode(latest, 1)
	if first == nil || !ix.aired(first) {
		return option{}, false
	}
	if days := daysBetween(first.AirDate, ix.now.Format("2006-01-02")); days < 0 || days > airingSeasonDays {
		return option{}, false
	}
	eps := ix.lookupAll(latest, c.eps)
	if eps == nil {
		return option{}, false
	}
	return option{eps: eps, weight: contraMin, method: MethodFolderSeason,
		reason: fmt.Sprintf("Episode %s of S%02d, airing now", n, latest)}, true
}

func (st *packState) dateOptions(c candidate) []option {
	ix := st.ix
	switch eps := ix.byDate[c.date]; {
	case len(eps) == 1:
		return []option{{eps: eps, weight: wExplicit, method: MethodDate,
			reason: fmt.Sprintf("Aired %s (%q in the name)", c.date, c.text)}}
	case len(eps) > 1:
		opts := make([]option, 0, len(eps))
		for _, ep := range eps {
			opts = append(opts, option{eps: []*Episode{ep}, weight: wNearDate, method: MethodDate,
				reason: fmt.Sprintf("Aired %s, like %d other episodes", c.date, len(eps)-1)})
		}
		return opts
	}
	var opts []option
	for _, d := range adjacentDates(c.date) {
		for _, ep := range ix.byDate[d] {
			opts = append(opts, option{eps: []*Episode{ep}, weight: wNearDate, method: MethodDate,
				reason: fmt.Sprintf("Aired %s, a day off the name's %s", d, c.date)})
		}
	}
	return opts
}

// specialOptions reads "OVA 2": the 2nd special whose name says OVA, else
// S00E02.
func (st *packState) specialOptions(c candidate) []option {
	ix := st.ix
	if !ix.hasSpecials || len(c.eps) == 0 {
		return nil
	}
	n := c.eps[0]
	var named []*Episode
	for _, ne := range ix.named {
		if ne.ep.Season != 0 {
			continue
		}
		for _, w := range ne.words {
			if w == c.word || c.word == "oad" && w == "ova" || c.word == "ova" && w == "oad" {
				named = append(named, ne.ep)
				break
			}
		}
	}
	sort.SliceStable(named, func(i, j int) bool { return named[i].Number < named[j].Number })
	if n <= len(named) {
		ep := named[n-1]
		return []option{{eps: []*Episode{ep}, weight: wSpecialNamed, method: MethodSpecialNumber,
			reason: fmt.Sprintf("%q → %s (%s special named %s)", c.text, seLabel([]*Episode{ep}), ordinal(n), strings.ToUpper(c.word))}}
	}
	if ep := ix.episode(0, n); ep != nil {
		return []option{{eps: []*Episode{ep}, weight: wSpecialMarker, method: MethodSpecialNumber,
			reason: fmt.Sprintf("%q read as %s", c.text, seLabel([]*Episode{ep}))}}
	}
	return nil
}

// titleOption finds the one episode whose name the file gives.
func (st *packState) titleOption(fi *fileInfo) (option, bool) {
	text := fi.titleText
	if !hasSignificant(text) {
		return option{}, false
	}
	type hit struct {
		ep  *Episode
		sim float64
	}
	var hits []hit
	for _, ne := range st.ix.named {
		if sim := nameSimilarity(text, ne.words); sim >= titleMin {
			hits = append(hits, hit{ne.ep, sim})
		}
	}
	if len(hits) == 0 {
		return option{}, false
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].sim > hits[j].sim })
	if len(hits) > 1 && hits[1].sim >= hits[0].sim {
		return option{}, false // two episodes fit equally well
	}
	ep := hits[0].ep
	return option{eps: []*Episode{ep}, weight: wTitle, method: MethodTitle,
		reason: fmt.Sprintf("Title %q → %s", clipRunes(ep.Name, 60), seLabel([]*Episode{ep}))}, true
}

// nameSimilarity compares the words a file gives for an episode's name with
// the name's words: 1 for the same words; 0.92 when the file has the whole
// name and more ("The Next Doctor Christmas Special"); 0.9 when it has the
// start of a longer name ("Ilse's Notebook" for "Ilse's Notebook: Memoirs
// of..."); otherwise the Dice coefficient of the two word lists.
func nameSimilarity(text, name []string) float64 {
	if len(text) == 0 || len(name) == 0 {
		return 0
	}
	if equalWords(text, name) {
		return 1
	}
	if (len(name) >= 2 || len(name[0]) >= 6) && containsPhrase(text, name) {
		return 0.92
	}
	if len(text) >= 2 && len(text) < len(name) && equalWords(name[:len(text)], text) {
		return 0.9
	}
	if len(text) >= 3 && containsPhrase(name, text) {
		return 0.9
	}
	counts := map[string]int{}
	for _, w := range name {
		counts[w]++
	}
	common := 0
	for _, w := range text {
		if counts[w] > 0 {
			counts[w]--
			common++
		}
	}
	return 2 * float64(common) / float64(len(text)+len(name))
}

func containsPhrase(words, phrase []string) bool {
	for i := 0; i+len(phrase) <= len(words); i++ {
		if equalWords(words[i:i+len(phrase)], phrase) {
			return true
		}
	}
	return false
}

// decision is the outcome of weighing a file's options.
type decision struct {
	best   *option
	score  float64
	margin float64
	note   string // agreement or contradiction, for the reason
}

// decide groups options by the episodes they point at and scores each
// answer: its strongest reading, +0.1 when an independent reading agrees,
// -0.3 when a strong reading (≥ 0.7) points elsewhere.
func decide(opts []option) decision {
	if len(opts) == 0 {
		return decision{}
	}
	type answer struct {
		key     string
		best    option
		methods map[Method]float64
		raw     float64 // with the agreement bonus, before any penalty
		score   float64
		note    string
	}
	var answers []*answer
	byKey := map[string]*answer{}
	for _, o := range opts {
		a := byKey[o.key()]
		if a == nil {
			a = &answer{key: o.key(), best: o, methods: map[Method]float64{}}
			byKey[a.key] = a
			answers = append(answers, a)
		}
		if o.weight > a.best.weight {
			a.best = o
		}
		a.methods[o.method] = max(a.methods[o.method], o.weight)
	}
	for _, a := range answers {
		a.raw = a.best.weight
		if m, ok := strongestOther(a.methods, a.best.method); ok {
			a.raw += bonusAgree
			a.note = methodLabel(m) + " agrees"
		}
		a.score = a.raw
		for _, b := range answers {
			if b == a || b.best.weight < contraMin {
				continue
			}
			a.score -= penaltyContra
			a.note = fmt.Sprintf("%s says %s", methodLabel(b.best.method), b.best.label())
			break
		}
		a.raw, a.score = min(a.raw, 1), min(a.score, 1)
	}
	sort.SliceStable(answers, func(i, j int) bool {
		if answers[i].score != answers[j].score {
			return answers[i].score > answers[j].score
		}
		return answers[i].best.weight > answers[j].best.weight
	})
	top := answers[0]
	d := decision{best: &top.best, score: top.score, margin: top.score, note: top.note}
	// The margin is over the runner-up's own evidence, before the penalty
	// the top answer's strength gave it: a strong reading must not make its
	// rivals look weaker than they are.
	for _, a := range answers[1:] {
		d.margin = min(d.margin, top.score-a.raw)
	}
	return d
}

// strongestOther returns the strongest agreeing method other than best, if
// one weighs at least agreeMin. Ties go to the method name, so reasons are
// stable.
func strongestOther(methods map[Method]float64, best Method) (Method, bool) {
	var found Method
	w := -1.0
	for m, mw := range methods {
		if m == best || mw < agreeMin {
			continue
		}
		if mw > w || mw == w && m < found {
			found, w = m, mw
		}
	}
	return found, w >= 0
}

// confidenceOf turns a score and margin into a confidence.
func confidenceOf(score, margin float64) Confidence {
	const eps = 1e-9
	switch {
	case score >= highMin-eps && margin >= highMargin-eps:
		return ConfidenceHigh
	case score >= mediumMin-eps:
		return ConfidenceMedium
	case score > 0:
		return ConfidenceLow
	}
	return ConfidenceNone
}

func capConfidence(c, limit Confidence) Confidence {
	if c.Rank() > limit.Rank() {
		return limit
	}
	return c
}

func methodLabel(m Method) string {
	switch m {
	case MethodSxxEyy, MethodNxNN, MethodSeasonNumber:
		return "the episode tag"
	case MethodFolderSeason:
		return "season numbering"
	case MethodAbsolute:
		return "absolute numbering"
	case MethodDate:
		return "the air date"
	case MethodTitle:
		return "the title"
	case MethodRemapGroup, MethodRemapCour, MethodRemapOverflow:
		return "the remap"
	case MethodSpecialNumber:
		return "the special number"
	case MethodConcatenated:
		return "the number"
	case MethodHint:
		return "the hint"
	}
	return string(m)
}

// lookupAll returns the episodes (season, e) for every e, or nil if any is
// missing.
func (ix *catalogIndex) lookupAll(season int, eps []int) []*Episode {
	out := make([]*Episode, 0, len(eps))
	for _, e := range eps {
		ep := ix.episode(season, e)
		if ep == nil {
			return nil
		}
		out = append(out, ep)
	}
	return out
}

// lookupAbs returns the episodes with absolute numbers nums, or nil if any
// is missing.
func (ix *catalogIndex) lookupAbs(nums []int) []*Episode {
	out := make([]*Episode, 0, len(nums))
	for _, n := range nums {
		ep := ix.byAbs[n]
		if ep == nil {
			return nil
		}
		out = append(out, ep)
	}
	return out
}

func (p *remapPlan) lookupAll(eps []int) []*Episode {
	out := make([]*Episode, 0, len(eps))
	for _, e := range eps {
		ep := p.lookup(e)
		if ep == nil {
			return nil
		}
		out = append(out, ep)
	}
	return out
}

func allInSeason(eps []*Episode, s int) bool {
	for _, ep := range eps {
		if ep.Season != s {
			return false
		}
	}
	return true
}

// seasonSource names where a file's season came from, for reasons.
func (st *packState) seasonSource(fi *fileInfo) string {
	switch {
	case fi.fileSeason >= 0:
		return fmt.Sprintf("Season %d in the name", fi.fileSeason)
	case st.dirSegs[fi.dir] != "":
		return fmt.Sprintf("Folder %q", clipRunes(st.dirSegs[fi.dir], 40))
	}
	return fmt.Sprintf("Hint season %d", st.hintSeason)
}

func numText(c candidate) string {
	if len(c.eps) == 1 {
		// The digits as written: "05" from "05v2" or "E05".
		start := strings.IndexFunc(c.text, func(r rune) bool { return r >= '0' && r <= '9' })
		if start >= 0 {
			end := start
			for end < len(c.text) && c.text[end] >= '0' && c.text[end] <= '9' {
				end++
			}
			return c.text[start:end]
		}
	}
	parts := make([]string, len(c.eps))
	for i, e := range c.eps {
		parts[i] = fmt.Sprintf("%02d", e)
	}
	return strings.Join(parts, "-")
}

// seLabel writes episodes as "S01E05" or "S01E05-E06" (or "S01E05+S02E01").
func seLabel(eps []*Episode) string {
	if len(eps) == 0 {
		return ""
	}
	first := eps[0]
	s := fmt.Sprintf("S%02dE%02d", first.Season, first.Number)
	if len(eps) == 1 {
		return s
	}
	last := eps[len(eps)-1]
	if last.Season == first.Season {
		return s + fmt.Sprintf("-E%02d", last.Number)
	}
	return s + fmt.Sprintf("+S%02dE%02d", last.Season, last.Number)
}

func seText(season int, eps []int) string {
	s := fmt.Sprintf("S%02dE%02d", season, eps[0])
	if len(eps) > 1 {
		s += fmt.Sprintf("-E%02d", eps[len(eps)-1])
	}
	return s
}

// adjacentDates returns the days before and after date.
func adjacentDates(date string) []string {
	var out []string
	for _, delta := range []int{-1, 1} {
		if d, ok := shiftDate(date, delta); ok {
			out = append(out, d)
		}
	}
	return out
}
