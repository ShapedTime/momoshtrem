package runs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
)

// The tools a run submits its output with.
const (
	NameSubmitShelf     = "submit_shelf"
	NameSubmitHeroPicks = "submit_hero_picks"
)

// What a run writes (PLAN §7 R2): 4–6 shelves of 6–15 titles, 5 asked
// for, and 3–9 hero picks.
const (
	ShelvesTarget = 5
	MinShelves    = 4
	MaxShelves    = 6
	MinHeroPicks  = recommend.HeroPicksPerDay
	MaxHeroPicks  = recommend.HeroPicksMax

	shelfTitleMaxChars    = 60
	shelfSubtitleMaxChars = 90
	kickerMaxChars        = 40
	slugMaxChars          = 40
)

var submitShelfSchema = compact(`{
	"type": "object",
	"properties": {
		"slug": {
			"description": "A short id for the shelf, lowercase with hyphens (because-severance, new-this-month). Submitting the same slug again replaces the shelf.",
			"type": "string"
		},
		"kind": {
			"description": "because: titles that share what the user loved in one title, named in the heading; new: released, or back with a new season, in the last few weeks; anime; gems: lesser-known titles with strong ratings; coming_soon: premieres in the next few months; other: any other theme.",
			"type": "string",
			"enum": ["because", "new", "anime", "gems", "coming_soon", "other"]
		},
		"title": {
			"description": "The shelf's heading, at most 60 characters: Because you loved Severance, New this month for you.",
			"type": "string"
		},
		"subtitle": {
			"description": "One short line under the heading, or empty for none.",
			"type": "string"
		},
		"items": {
			"description": "6–15 titles, best first.",
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"media_type": {"type": "string", "enum": ["movie", "tv"]},
					"tmdb_id": {"type": "integer"},
					"title": {
						"description": "The title as TMDB names it: the server checks the id against it.",
						"type": "string"
					},
					"year": {
						"description": "Its release (or first air) year, as TMDB has it: the server checks the id against it too. 0 when unsure.",
						"type": "integer"
					},
					"reason": {
						"description": "Why this user will like it, in one line of at most 140 characters, grounded in their profile (a title they loved, a creator, a theme, a habit). No spoilers, no generic praise.",
						"type": "string"
					}
				},
				"required": ["media_type", "tmdb_id", "title", "year", "reason"],
				"additionalProperties": false
			}
		}
	},
	"required": ["slug", "kind", "title", "subtitle", "items"],
	"additionalProperties": false
}`)

var submitHeroPicksSchema = compact(`{
	"type": "object",
	"properties": {
		"picks": {
			"description": "3–9 titles, strongest first.",
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"media_type": {"type": "string", "enum": ["movie", "tv"]},
					"tmdb_id": {"type": "integer"},
					"title": {
						"description": "The title as TMDB names it: the server checks the id against it.",
						"type": "string"
					},
					"year": {
						"description": "Its release (or first air) year, as TMDB has it: the server checks the id against it too. 0 when unsure.",
						"type": "integer"
					},
					"kicker": {
						"description": "A short line above the title, at most 40 characters: A slow-burn mystery, From the makers of Dark.",
						"type": "string"
					},
					"reason": {
						"description": "Why tonight, in one line of at most 140 characters, grounded in their profile.",
						"type": "string"
					}
				},
				"required": ["media_type", "tmdb_id", "title", "year", "kicker", "reason"],
				"additionalProperties": false
			}
		}
	},
	"required": ["picks"],
	"additionalProperties": false
}`)

// shelf is a shelf the agent submitted, as it was saved.
type shelf struct {
	slug, kind, title string
	subtitle          *string
	items             []entry
}

// entry is a title that passed validation, with its snapshot.
type entry struct {
	ref     library.TitleRef
	reason  string
	kicker  string // hero picks only
	meta    recommend.ItemMeta
	signals json.RawMessage
}

// board holds a run's output as the agent submits it. Tools run one at a
// time on the run's goroutine, so it needs no lock.
type board struct {
	v *validator
	// pool marks the titles that came from the candidate pool, with their
	// rank and sources (rec_items.signals).
	pool map[library.TitleRef]poolMark
	// onShelves follows shelves_done.
	onShelves func(done int)

	shelves []*shelf // in the order first submitted; a resubmitted slug keeps its place
	picks   []entry
}

type poolMark struct {
	rank int
	why  []string
}

func newBoard(v *validator, pool *candidates.Pool, onShelves func(int)) *board {
	b := &board{v: v, pool: map[library.TitleRef]poolMark{}, onShelves: onShelves}
	if pool != nil {
		for i, c := range pool.Items {
			b.pool[c.Ref()] = poolMark{rank: i + 1, why: c.Why}
		}
	}
	return b
}

// tools are the two submit tools over the board.
func (b *board) tools() []agent.Tool {
	return []agent.Tool{
		&submitTool{
			name: NameSubmitShelf,
			description: "Save one shelf of For You. Call it once per shelf, as soon as that shelf is ready: the user watches the shelves " +
				"come in (2 of 5). The server checks every title: it must exist on TMDB under the title and year you give, and not be watched, " +
				"started, rated, hidden or dropped on Trakt, or turned down; the result lists what it dropped and why. A shelf left with " +
				"fewer than 6 titles isn't saved. Submitting a slug again replaces that shelf, dropped titles and all: use it to replace " +
				"them. At most 6 shelves.",
			schema: submitShelfSchema,
			run:    b.submitShelf,
		},
		&submitTool{
			name: NameSubmitHeroPicks,
			description: "Save Tonight's pick: 3–9 titles, strongest first, each with a kicker and a reason. For You shows three a day " +
				"from this list, so it should hold up over several days. The server checks the titles as it does a shelf's; a call " +
				"replaces the picks saved before.",
			schema: submitHeroPicksSchema,
			run:    b.submitHeroPicks,
		},
	}
}

// done is how many shelves are saved.
func (b *board) done() int { return len(b.shelves) }

type shelfInput struct {
	Slug     string       `json:"slug"`
	Kind     string       `json:"kind"`
	Title    string       `json:"title"`
	Subtitle string       `json:"subtitle"`
	Items    []titleInput `json:"items"`
}

type titleInput struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	Title     string `json:"title"`
	Year      int    `json:"year"` // 0: unsure
	Kicker    string `json:"kicker"`
	Reason    string `json:"reason"`
}

// drop is a title the server left out, as the model reads it.
type drop struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	Title     string `json:"title"`
	Why       string `json:"why"`
}

var shelfKinds = map[string]bool{
	recommend.ShelfBecause: true, recommend.ShelfNew: true, recommend.ShelfAnime: true,
	recommend.ShelfGems: true, recommend.ShelfComingSoon: true, recommend.ShelfOther: true,
}

func (b *board) submitShelf(ctx context.Context, input json.RawMessage) (any, error) {
	var in shelfInput
	if err := decode(input, &in); err != nil {
		return nil, err
	}
	if !shelfKinds[in.Kind] {
		return nil, &agent.ToolError{Message: "kind is because, new, anime, gems, coming_soon or other."}
	}
	title := clip(strings.TrimSpace(in.Title), shelfTitleMaxChars)
	if title == "" {
		return nil, &agent.ToolError{Message: "The shelf needs a title."}
	}
	slug := slugify(in.Slug)
	if slug == "" {
		slug = slugify(title)
	}
	at := -1
	for i, s := range b.shelves {
		if s.slug == slug {
			at = i
		}
	}
	if at < 0 && len(b.shelves) >= MaxShelves {
		return nil, &agent.ToolError{Message: fmt.Sprintf("There are already %d shelves; submit one of their slugs (%s) to replace it.",
			MaxShelves, strings.Join(b.slugs(), ", "))}
	}
	if len(in.Items) == 0 {
		return nil, &agent.ToolError{Message: "The shelf has no titles: send 6–15."}
	}

	// Titles on another shelf, kept apart so each shelf brings something new.
	elsewhere := map[library.TitleRef]string{}
	for i, s := range b.shelves {
		if i == at {
			continue
		}
		for _, e := range s.items {
			elsewhere[e.ref] = s.slug
		}
	}
	kept, drops, err := b.validate(ctx, in.Items, in.Kind == recommend.ShelfComingSoon, elsewhere, false)
	if err != nil {
		return nil, err
	}
	for i := recommend.ShelfMaxItems; i < len(kept); i++ {
		drops = append(drops, drop{MediaType: kept[i].ref.MediaType, TMDBID: kept[i].ref.TMDBID, Title: kept[i].meta.Title,
			Why: fmt.Sprintf("over %d titles", recommend.ShelfMaxItems)})
	}
	kept = kept[:min(len(kept), recommend.ShelfMaxItems)]
	if len(kept) < recommend.ShelfMinItems {
		return nil, &agent.ToolError{Message: fmt.Sprintf(
			"The shelf %q wasn't saved: only %d of its titles can be picked, and a shelf needs %d–%d. Submit it again with replacements for these: %s",
			slug, len(kept), recommend.ShelfMinItems, recommend.ShelfMaxItems, encodeDrops(drops))}
	}

	s := &shelf{slug: slug, kind: in.Kind, title: title, items: kept}
	if sub := clip(strings.TrimSpace(in.Subtitle), shelfSubtitleMaxChars); sub != "" {
		s.subtitle = &sub
	}
	if at >= 0 {
		b.shelves[at] = s
	} else {
		b.shelves = append(b.shelves, s)
	}
	if b.onShelves != nil {
		b.onShelves(len(b.shelves))
	}
	res := struct {
		Saved       string `json:"saved"`
		Titles      int    `json:"titles"`
		ShelvesDone int    `json:"shelves_done"`
		Dropped     []drop `json:"dropped"`
		Note        string `json:"note,omitempty"`
	}{Saved: slug, Titles: len(kept), ShelvesDone: len(b.shelves), Dropped: drops}
	if len(drops) > 0 {
		res.Note = "Saved without the dropped titles. To replace them, submit this slug again with the full list."
	}
	return res, nil
}

func (b *board) submitHeroPicks(ctx context.Context, input json.RawMessage) (any, error) {
	var in struct {
		Picks []titleInput `json:"picks"`
	}
	if err := decode(input, &in); err != nil {
		return nil, err
	}
	if len(in.Picks) == 0 {
		return nil, &agent.ToolError{Message: fmt.Sprintf("Send %d–%d picks.", MinHeroPicks, MaxHeroPicks)}
	}
	kept, drops, err := b.validate(ctx, in.Picks, false, nil, true)
	if err != nil {
		return nil, err
	}
	for i := MaxHeroPicks; i < len(kept); i++ {
		drops = append(drops, drop{MediaType: kept[i].ref.MediaType, TMDBID: kept[i].ref.TMDBID, Title: kept[i].meta.Title,
			Why: fmt.Sprintf("over %d picks", MaxHeroPicks)})
	}
	kept = kept[:min(len(kept), MaxHeroPicks)]
	if len(kept) < MinHeroPicks {
		return nil, &agent.ToolError{Message: fmt.Sprintf(
			"The hero picks weren't saved: only %d can be picked, and they need %d–%d. Submit them again with replacements for these: %s",
			len(kept), MinHeroPicks, MaxHeroPicks, encodeDrops(drops))}
	}
	b.picks = kept
	return struct {
		Saved   int    `json:"saved"`
		Dropped []drop `json:"dropped"`
	}{Saved: len(kept), Dropped: drops}, nil
}

// validate checks submitted titles: a reason (and a kicker for a hero
// pick), each once, none from another shelf, then the exclusion rule and
// TMDB. Kept titles stay in order.
func (b *board) validate(ctx context.Context, in []titleInput, comingSoon bool, elsewhere map[library.TitleRef]string,
	hero bool) ([]entry, []drop, error) {
	var (
		drops  []drop
		checks []proposed
		inputs []titleInput
		seen   = map[library.TitleRef]bool{}
	)
	for _, t := range in {
		ref := library.TitleRef{MediaType: t.MediaType, TMDBID: t.TMDBID}
		d := drop{MediaType: t.MediaType, TMDBID: t.TMDBID, Title: t.Title}
		switch {
		case strings.TrimSpace(t.Reason) == "":
			d.Why = "no reason"
		case hero && strings.TrimSpace(t.Kicker) == "":
			d.Why = "no kicker"
		case seen[ref]:
			d.Why = "listed twice"
		case elsewhere[ref] != "":
			d.Why = "already on the shelf " + elsewhere[ref]
		}
		seen[ref] = true
		if d.Why != "" {
			drops = append(drops, d)
			continue
		}
		checks = append(checks, proposed{Ref: ref, Title: t.Title, Year: t.Year})
		inputs = append(inputs, t)
	}
	verdicts, err := b.v.check(ctx, checks, comingSoon)
	if err != nil {
		return nil, nil, err
	}
	var kept []entry
	for i, v := range verdicts {
		t := inputs[i]
		if v.Dropped != "" {
			drops = append(drops, drop{MediaType: t.MediaType, TMDBID: t.TMDBID, Title: t.Title, Why: v.Dropped})
			continue
		}
		e := entry{ref: v.Ref, reason: clip(strings.TrimSpace(t.Reason), recommend.ReasonMaxChars), meta: v.Meta,
			signals: b.signals(v.Ref)}
		if hero {
			e.kicker = clip(strings.TrimSpace(t.Kicker), kickerMaxChars)
		}
		kept = append(kept, e)
	}
	if drops == nil {
		drops = []drop{}
	}
	return kept, drops, nil
}

// signals say where a title came from (rec_items.signals, for debugging):
// its rank and sources in the pool, or not from the pool (a search, the
// web, the model's own knowledge).
func (b *board) signals(ref library.TitleRef) json.RawMessage {
	var v any = map[string]any{"pool": false}
	if m, ok := b.pool[ref]; ok {
		v = map[string]any{"pool": true, "rank": m.rank, "why": m.why}
	}
	out, err := tools.Encode(v)
	if err != nil {
		return nil
	}
	return out
}

func (b *board) slugs() []string {
	out := make([]string, len(b.shelves))
	for i, s := range b.shelves {
		out[i] = s.slug
	}
	return out
}

// rows are the board's shelves and hero picks as the store writes them.
func (b *board) rows() ([]library.RecShelfRow, []library.RecHeroPickRow, error) {
	shelves := make([]library.RecShelfRow, 0, len(b.shelves))
	for _, s := range b.shelves {
		row := library.RecShelfRow{Slug: s.slug, Kind: s.kind, Title: s.title, Subtitle: s.subtitle}
		for _, e := range s.items {
			meta, err := json.Marshal(e.meta)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to encode %s %d's snapshot: %w", e.ref.MediaType, e.ref.TMDBID, err)
			}
			row.Items = append(row.Items, library.RecItemRow{TitleRef: e.ref, Reason: e.reason, Meta: meta, Signals: e.signals})
		}
		shelves = append(shelves, row)
	}
	picks := make([]library.RecHeroPickRow, 0, len(b.picks))
	for _, e := range b.picks {
		meta, err := json.Marshal(e.meta)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to encode %s %d's snapshot: %w", e.ref.MediaType, e.ref.TMDBID, err)
		}
		picks = append(picks, library.RecHeroPickRow{TitleRef: e.ref, Kicker: e.kicker, Reason: e.reason, Meta: meta})
	}
	return shelves, picks, nil
}

func encodeDrops(drops []drop) string {
	out, err := tools.Encode(drops)
	if err != nil {
		return "[]"
	}
	return string(out)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugify makes a slug: lowercase letters and digits joined by hyphens.
func slugify(s string) string {
	s = nonSlug.ReplaceAllString(normalizeTitle(s), "-")
	s = strings.Trim(s, "-")
	if len(s) > slugMaxChars {
		cut := s[:slugMaxChars]
		if s[slugMaxChars] != '-' { // mid-word: back to the last whole one
			if i := strings.LastIndexByte(cut, '-'); i > 0 {
				cut = cut[:i]
			}
		}
		s = strings.TrimRight(cut, "-")
	}
	return s
}

// clip cuts s to at most n characters, at a word when one ends in its
// second half, with an ellipsis.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)[:n-1]
	cut := string(r)
	if i := strings.LastIndexByte(cut, ' '); i > len(cut)/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.-–—") + "…"
}

// submitTool is a submit tool: a fixed definition over a run function.
type submitTool struct {
	name, description string
	schema            json.RawMessage
	run               func(ctx context.Context, input json.RawMessage) (any, error)
}

func (t *submitTool) Name() string            { return t.name }
func (t *submitTool) Description() string     { return t.description }
func (t *submitTool) Schema() json.RawMessage { return t.schema }

// Run executes the tool and encodes its result compactly.
func (t *submitTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	out, err := t.run(ctx, input)
	if err != nil {
		return nil, err
	}
	b, err := tools.Encode(out)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to encode the result: %w", t.name, err)
	}
	return b, nil
}

// compact turns a schema written for people into the bytes sent, once.
func compact(schema string) json.RawMessage {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(schema)); err != nil {
		panic(fmt.Sprintf("invalid tool schema: %v", err))
	}
	return buf.Bytes()
}

// decode reads a tool's input. Strict tool use makes it match the schema,
// so a failure is the model's to see.
func decode(input json.RawMessage, v any) error {
	if len(bytes.TrimSpace(input)) == 0 {
		input = json.RawMessage(`{}`)
	}
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return &agent.ToolError{Message: "The input doesn't match the tool's schema: " + err.Error()}
	}
	return nil
}
