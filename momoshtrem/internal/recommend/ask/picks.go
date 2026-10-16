package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// NameShowPicks is the tool that shows the user Ask's answer: the intro,
// the picks and the refinement chips.
const NameShowPicks = "show_picks"

// Limits on what show_picks shows (PLAN §6).
const (
	itemsMax         = 10 // a turn's picks
	refinementsChars = 40
)

// showPicksSchema is strict and has no union: every property is required,
// and "nothing" is "" or [] (the request's 16 union parameters are shared
// by every strict tool).
var showPicksSchema = compactSchema(`{
	"type": "object",
	"properties": {
		"intro": {
			"description": "One or two plain sentences that open your answer, shown above the picks. Empty when you call it again with replacements.",
			"type": "string"
		},
		"items": {
			"description": "Your picks, best first: at most 10 in a turn, across calls.",
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"media_type": {"type": "string", "enum": ["movie", "tv"]},
					"tmdb_id": {
						"description": "The title's TMDB id, as a tool answered it.",
						"type": "integer"
					},
					"title": {
						"description": "The title as TMDB names it: the server checks the id against it.",
						"type": "string"
					},
					"year": {
						"description": "Its release (or first air) year, as TMDB has it: the server checks the id against it too. 0 when unsure.",
						"type": "integer"
					},
					"match_tag": {
						"description": "What it matches, up to 32 characters: \"Mystery + workplace · lighter\".",
						"type": "string"
					},
					"reason": {
						"description": "Why they'll like it, up to 140 characters, grounded in their taste profile.",
						"type": "string"
					}
				},
				"required": ["media_type", "tmdb_id", "title", "year", "match_tag", "reason"],
				"additionalProperties": false
			}
		},
		"refinements": {
			"description": "2 to 5 follow-ups they might tap next, a few words each (\"Lighter\", \"Only anime\"). When you call it again with replacements, [] keeps the ones you gave.",
			"type": "array",
			"items": {"type": "string"}
		}
	},
	"required": ["intro", "items", "refinements"],
	"additionalProperties": false
}`)

const showPicksDescription = "Show the user your answer: the intro, your picks and refinement chips. It's the only way titles reach them. " +
	"Each pick is checked and appears as a card at once; one that's watched, started, rated, dropped, hidden, turned down, " +
	"unknown to TMDB, named by another title's id, already shown or past the 10th is dropped. " +
	"The answer lists what was shown and what was dropped and why."

type picksInput struct {
	Intro string `json:"intro"`
	Items []struct {
		MediaType string `json:"media_type"`
		TMDBID    int    `json:"tmdb_id"`
		Title     string `json:"title"`
		Year      int    `json:"year"`
		MatchTag  string `json:"match_tag"`
		Reason    string `json:"reason"`
	} `json:"items"`
	Refinements []string `json:"refinements"`
}

// pickRef is a title in show_picks' answer.
type pickRef struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	Title     string `json:"title,omitempty"`
	Why       string `json:"why,omitempty"` // a dropped pick's
}

type picksResult struct {
	Shown   []pickRef `json:"shown"`
	Dropped []pickRef `json:"dropped"`
	Total   int       `json:"total_shown"` // this turn, across calls
	Room    int       `json:"room"`        // picks still allowed this turn
	Next    string    `json:"next"`
}

// Why a pick was dropped.
const (
	dropInvalid   = "not a movie or show id"
	dropDuplicate = "already shown"
	dropFull      = "past the 10th pick"
	dropUnknown   = "TMDB has no such title"
	dropNoAnswer  = "TMDB didn't answer; try another"
)

// picksTool is show_picks, bound to a turn; its definition is the same for
// every turn (it sits in the cached prefix).
type picksTool struct {
	run func(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

func (picksTool) Name() string            { return NameShowPicks }
func (picksTool) Description() string     { return showPicksDescription }
func (picksTool) Schema() json.RawMessage { return showPicksSchema }

func (p picksTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if p.run == nil {
		return nil, errors.New("show_picks isn't bound to a turn")
	}
	return p.run(ctx, input)
}

var _ agent.Tool = picksTool{}

// showPicks runs a show_picks call: the intro first (a text event), then
// each pick that passes, resolved on TMDB and sent at once (an item
// event), then the refinements kept for done.
func (t *turn) showPicks(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in picksInput
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return nil, &agent.ToolError{Message: "The input doesn't match show_picks' schema: " + err.Error()}
	}
	t.reach(recommend.StepWrite)
	if intro := strings.TrimSpace(in.Intro); intro != "" && t.intro == "" {
		t.emitText(intro)
	}

	res := picksResult{Shown: []pickRef{}, Dropped: []pickRef{}}
	drop := func(mediaType string, id int, why string) {
		res.Dropped = append(res.Dropped, pickRef{MediaType: mediaType, TMDBID: id, Why: why})
	}
	type pick struct {
		ref              library.TitleRef
		title            string
		year             int
		matchTag, reason string
	}
	var picks []pick
	seen := map[library.TitleRef]bool{}
	for _, it := range t.items {
		seen[it.ref()] = true
	}
	for _, it := range in.Items {
		ref := library.TitleRef{MediaType: it.MediaType, TMDBID: it.TMDBID}
		switch {
		case (ref.MediaType != library.TitleMovie && ref.MediaType != library.TitleTV) || ref.TMDBID <= 0 || ref.TMDBID > math.MaxInt32:
			drop(it.MediaType, it.TMDBID, dropInvalid)
		case seen[ref]:
			drop(it.MediaType, it.TMDBID, dropDuplicate)
		default:
			seen[ref] = true
			picks = append(picks, pick{ref: ref, title: it.Title, year: it.Year, matchTag: clip(it.MatchTag, recommend.MatchTagMaxChars),
				reason: clip(it.Reason, recommend.ReasonMaxChars)})
		}
	}

	refs := make([]library.TitleRef, len(picks))
	for i, p := range picks {
		refs[i] = p.ref
	}
	l, err := readLive(ctx, t.s.d.Titles, t.s.d.Library, refs)
	if err != nil {
		return nil, err
	}
	today := t.s.d.Now()
	for _, p := range picks {
		if len(t.items) >= itemsMax {
			drop(p.ref.MediaType, p.ref.TMDBID, dropFull)
			continue
		}
		if why := l.excluded(p.ref); why != "" {
			drop(p.ref.MediaType, p.ref.TMDBID, why)
			continue
		}
		meta, names, err := metaOf(ctx, t.s.d.TMDB, p.ref, today)
		switch {
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case errors.Is(err, tmdb.ErrNotFound):
			drop(p.ref.MediaType, p.ref.TMDBID, dropUnknown)
			continue
		case err != nil:
			t.s.log.Warn("Ask: TMDB didn't describe a pick", "media_type", p.ref.MediaType, "tmdb_id", p.ref.TMDBID, "error", err)
			drop(p.ref.MediaType, p.ref.TMDBID, dropNoAnswer)
			continue
		}
		if year := deref(meta.Year); !recommend.SameTitle(p.title, names...) || !recommend.SameYear(p.year, year) {
			// An id remembered for another title: its card would show that one
			drop(p.ref.MediaType, p.ref.TMDBID, mismatch(p.title, p.year, meta.Title, year))
			continue
		}
		item := viewItem{MediaType: p.ref.MediaType, TMDBID: p.ref.TMDBID, MatchTag: p.matchTag, Reason: p.reason, Meta: meta}
		t.items = append(t.items, item)
		t.tracker.tally.picks++
		res.Shown = append(res.Shown, pickRef{MediaType: item.MediaType, TMDBID: item.TMDBID, Title: meta.Title})
		t.send(recommend.Event{Name: recommend.EventItem, Data: l.item(item)})
	}
	if refinements := cleanRefinements(in.Refinements); len(refinements) > 0 {
		t.refinements = refinements
	}

	res.Total, res.Room = len(t.items), itemsMax-len(t.items)
	switch {
	case len(res.Dropped) == 0:
		res.Next = "They're on screen. End your turn now with a short closing line."
	case res.Room == 0:
		res.Next = "Dropped picks weren't shown, and the turn has its 10. End your turn now with a short closing line."
	default:
		res.Next = "Dropped picks weren't shown. Call show_picks again with replacements and an empty intro if you like, then end your turn with a short closing line."
	}
	return tools.Encode(res)
}

// mismatch is why a pick whose id names another title was dropped, worded
// as R2's (runs/validate.go), so the model looks the title up.
func mismatch(given string, givenYear int, actual string, year int) string {
	name, want := actual, given
	if year > 0 {
		name = fmt.Sprintf("%s (%d)", actual, year)
	}
	if givenYear > 0 {
		want = fmt.Sprintf("%s (%d)", given, givenYear)
	}
	return fmt.Sprintf("that tmdb_id is “%s” on TMDB, not “%s”: look the title up with tmdb_search", name, want)
}

func deref(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

// cleanRefinements keeps up to 5 distinct, non-empty chips of a few words.
func cleanRefinements(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range in {
		r = clip(strings.Join(strings.Fields(r), " "), refinementsChars)
		key := strings.ToLower(r)
		if r == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
		if len(out) == recommend.RefinementsMax {
			break
		}
	}
	return out
}

// clip trims s and cuts it to n characters, ending in "…" when cut.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRight(string(r[:n-1]), " ") + "…"
}

// compactSchema turns a schema written for people into the bytes sent.
func compactSchema(schema string) json.RawMessage {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(schema)); err != nil {
		panic(fmt.Sprintf("invalid tool schema: %v", err))
	}
	return buf.Bytes()
}
