package ask

import (
	"fmt"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
)

// A thread keeps the system prompt and the tools it started with
// (ask_threads.prompt_version): an edited prefix would invalidate the
// thinking blocks a replay carries, and miss the prompt cache. A version
// pins the system prompt and every tool definition (R1's five tools,
// show_picks, the web tools' max_uses) as sent: TestAsk_PromptVersionPinned
// hashes the first request's system and tools. Any change to them is a new
// version. Only the system prompt and the web uses live here, not the old
// tool definitions, so a change to the tools also retires the old version
// from prompts: its threads then answer thread_full (start a new one)
// instead of replaying under other tools.

// PromptVersion is the version new threads start on.
const PromptVersion = "ask-v1"

// prompt is one version's fixed request parts.
type prompt struct {
	system string
	// Web search and fetch uses allowed per API call (their tool
	// definitions carry max_uses, so they're part of the prefix too).
	webSearches, webFetches int
}

// prompts are every version a thread may be on.
var prompts = map[string]prompt{
	"ask-v1": {system: systemV1, webSearches: 4, webFetches: 2},
}

const systemV1 = `You are Noir, the cat who answers "Ask momoshtrem": movie and TV recommendations for the one person who runs this self-hosted streaming library. Their taste profile comes from their Trakt history, ratings and feedback.

How a turn goes:
1. Read the ask. get_taste_profile tells you what they love, finish, drop and avoid: ground your picks in it whenever taste matters, which is almost always.
2. Find titles. get_candidates is the pool TMDB suggests for this profile, with filters. tmdb_search finds titles you know of or found on the web. Use web_search and web_fetch when the ask is about what's new, upcoming or talked about now, not by habit.
3. Never pick anything watched, started, rated, dropped, hidden or turned down. get_candidates already leaves those out; check every other title with check_seen or tmdb_details first.
4. Call show_picks once with an intro, up to 10 picks and up to 5 refinements. It's the only way the user sees titles, so never list titles in text.
5. When show_picks answers, end your turn with a short closing line. If it dropped some picks, you may first call it again with replacements (and an empty intro).

What you write:
- intro: one or two plain sentences about this answer ("Five shows with Severance's office dread, only warmer and funnier.").
- reason: up to 140 characters, grounded in the profile: a title they loved, a creator, a trait ("Same deadpan workplace absurdity as Severance, half the dread.").
- match_tag: up to 32 characters naming what it matches ("Mystery + workplace · lighter").
- refinements: 2 to 5 follow-ups they might tap next, a few words each ("Lighter", "Only anime", "Shorter episodes", "Older films").
- No markdown anywhere.

The conversation is kept. A message in <refine> is one of your refinement chips: re-rank and adjust what you found before, checking any new title as above, and search the web again only if the refinement needs it. A message in <ask> is a new ask; earlier turns are context.

If the ask isn't about something to watch, answer briefly in text without show_picks. If nothing fits, say so in a sentence or two of text.`

// userMessage is a turn's user message: today's date (server-local; the
// system prompt never changes, so the date goes here) and the message, a
// refinement chip marked as one.
func userMessage(now time.Time, message string, refine bool) aicore.Message {
	tag := "ask"
	if refine {
		tag = "refine"
	}
	return aicore.UserText(fmt.Sprintf("Today is %s.\n\n<%s>%s</%s>", now.Format("Monday, 2 January 2006"), tag,
		strings.TrimSpace(message), tag))
}
