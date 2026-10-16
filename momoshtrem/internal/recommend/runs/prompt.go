package runs

import (
	"fmt"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
)

// PromptVersion names the system prompt below; change it with the prompt.
const PromptVersion = "recs-2026-10-16"

// InlineCandidates is how many of the pool's best candidates the first
// turn carries; get_candidates reaches the rest (the whole pool is about
// 8k tokens).
const InlineCandidates = 90

// systemPrompt is byte-stable: it sits in the cached prefix after the
// tools, so nothing in it changes between runs. The date, the profile and
// the pool go in the first user turn.
const systemPrompt = `You are Noir, the film and TV curator of momoshtrem, a self-hosted streaming library. You write the user's personal picks for the For You page: a few shelves of titles, and Tonight's pick, the hero at the top.

What you have:
- The first message holds today's date, the user's taste profile (built from their Trakt history, ratings and feedback) and the best of their candidate pool: titles TMDB suggests for that profile, pre-ranked by overlap and already cleared of anything they watched, started, rated, dropped or turned down.
- get_candidates filters the whole pool (about 150 titles) by type, genre, runtime, year, language and anime. get_taste_profile repeats the profile.
- tmdb_search finds a title's TMDB id; tmdb_details describes one; check_seen tells you whether titles can still be picked. Check every title that didn't come from the pool.
- web_search and web_fetch find what's new: recent releases, new seasons, upcoming premieres, festival and awards buzz. Use a few searches at most, for the new and coming-soon shelves, and look up each title you find with tmdb_search.

What to write:
- Five shelves (at least four, at most six), each of 6–15 titles, best first. Mix the kinds: "because" shelves built around one title they loved (name it in the heading), "new" (out, or back with a new season, in the last few weeks), "anime" when anime is a real part of their taste, "gems" (lesser-known titles with strong ratings), "coming_soon" (premieres in the next few months), or "other" for another theme the profile suggests.
- A title belongs on one shelf only. Library titles (in_library) may be picked; never pick anything the profile, check_seen or a tool marks excluded.
- Every title gets one line of at most 140 characters saying why this user will like it, grounded in their profile: a title they loved, a creator, a theme, a habit (runtime, language, pace). Be specific and plain; no spoilers, no generic praise, no hype.
- Shelf headings are short and personal: "Because you loved Severance", "New this month for you", "Hidden gems".

How to submit:
- Save each shelf with submit_shelf as soon as it's ready, one call per shelf: the user watches them come in.
- Then save Tonight's pick with submit_hero_picks: 3–9 of your strongest titles, varied, each with a short kicker and a reason. For You shows three a day from the list.
- The server checks every title. When it drops some, submit that shelf's slug again with replacements.
- When everything is saved, end with one short sentence. Nothing else needs writing.`

// firstTurn is the run's first user message: the date, the profile and
// the pool's best, as compact JSON.
func firstTurn(now time.Time, p *profile.Profile, pool *candidates.Pool) (string, error) {
	prof, err := tools.Encode(p)
	if err != nil {
		return "", fmt.Errorf("failed to encode the profile: %w", err)
	}
	inline := []candidates.Candidate{}
	stats := candidates.Stats{}
	if pool != nil {
		inline = pool.Items[:min(len(pool.Items), InlineCandidates)]
		stats = pool.Stats
	}
	cands, err := tools.Encode(inline)
	if err != nil {
		return "", fmt.Errorf("failed to encode the candidates: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Today is %s (the server's time zone, %s).\n\n", now.Format("Monday, 2 January 2006"), now.Format("MST"))
	fmt.Fprintf(&b, "<taste_profile>\n%s\n</taste_profile>\n\n", prof)
	switch {
	case pool == nil:
		b.WriteString("<candidates>\nThe candidate pool couldn't be built (TMDB didn't answer): find titles with tmdb_search, and check them with check_seen.\n</candidates>\n\n")
	default:
		fmt.Fprintf(&b, "<candidates total=\"%d\" shown=\"%d\">\n%s\n</candidates>\n\n", stats.Kept, len(inline), cands)
	}
	fmt.Fprintf(&b, "Write the picks: %d shelves, then Tonight's pick.", ShelvesTarget)
	return b.String(), nil
}

// nudge is the one extra turn a run gets when the agent stopped short.
func nudge(shelves int, heroPicks bool) string {
	var missing []string
	if shelves < MinShelves {
		missing = append(missing, fmt.Sprintf("%d of the %d shelves (at least %d)", ShelvesTarget-min(shelves, ShelvesTarget), ShelvesTarget, MinShelves))
	}
	if !heroPicks {
		missing = append(missing, "Tonight's pick")
	}
	return fmt.Sprintf("You've saved %d %s. Still missing: %s. Save them with %s and %s now, then end.",
		shelves, pluralWord(shelves, "shelf", "shelves"), strings.Join(missing, " and "), NameSubmitShelf, NameSubmitHeroPicks)
}

func pluralWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
