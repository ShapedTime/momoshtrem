import type {
  AiFeature,
  AiUsage,
  AskErrorCode,
  AskEvent,
  AskItem,
  AskStep,
  AskStepId,
  AskThread,
  AskThreadSummary,
  AskTurn,
  AskTurnStatus,
  AskTurnUsage,
  HeroPick,
  MediaType,
  RecFeedbackSource,
  RecItem,
  RecModel,
  RecommendationsHome,
  RecRunKind,
  RecRunStatus,
  RecSchedule,
  RecSignal,
  RecSkipReason,
  RecUsage,
  RunDetail,
  RunProgress,
  RunStep,
  RunSummary,
  Shelf,
  ShelfKind,
} from '@/lib/api/contract';
import { applyStatus } from '@/lib/recommendations/steps';
import { type MockScenario, scenario } from '../scenario';
import { findTitle } from './catalog';
import { AI_SPEND, aiConfigured, mockAiSettings } from './settings';
// titles.ts reads recommendationReason() back, and trakt.ts imports titles.ts: functions only, so the cycles are safe
import { buildTitle, type MockLibrary } from './titles';
import { traktImported, watchlistItems } from './trakt';

/**
 * Recommendations and Ask momoshtrem (PLAN §6 "Recommendations (R1–R3)"),
 * as a state machine per page:
 * - **Runs.** Refresh queues a run; each read of /home (or of the run) moves
 *   it one step: history, web, TMDB, then the shelves two at a time. On
 *   success /home serves its picks (the design's set and a second one, in
 *   turn). Scenarios start from a fixed state: `recs-*` (scenario.ts).
 * - **Feedback** is kept per title: negative signals hide it from the
 *   shelves, the hero, thread reads and the title page's reason at once;
 *   `like` shows on the cards.
 * - **Ask** streams a turn picked from the message (the design's sets:
 *   lighter Severance, a 90-minute thriller, anime, films for parents) or a
 *   refinement chip, and the thread grows. `ask-*` scenarios fail or hold it.
 * - **Spend**: what runs and asks cost on the page adds to the month's
 *   (fixtures/settings.ts), so the budget and usage move.
 * The titles' facts are snapshots, as the backend stores them; library state
 * and the watchlist come from the page's library and Trakt mocks.
 */

const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

export const ASK_DAILY_LIMIT = 30;
export const THREAD_MAX_TURNS = 20;
const THREADS_LISTED = 50;
/** A new thread's title (PLAN §6): at most 60 characters, cut back to a space past index 30, then "…". */
const THREAD_TITLE_MAX = 60;
const THREAD_TITLE_MIN_CUT = 30;
const HERO_PER_DAY = 3;
const SHELVES_TARGET = 5;
/** Reads a run takes: queued → history → web → tmdb → writing (2 of 5) → writing (4 of 5) → done. */
export const RUN_DONE_TICK = 6;
/** `recs-failed`: a run fails when it reaches TMDB. */
const RUN_FAIL_TICK = 3;
/** `recs-first-run` and `recs-refreshing` hold here: "Writing your shelves", 2 of 5. */
const RUN_HOLD_TICK = 4;
const RUN_ERROR = 'The Anthropic API didn’t answer (overloaded). Your last picks stay; try again in a few minutes.';

const NEGATIVE: ReadonlySet<RecSignal> = new Set(['dislike', 'not_interested', 'seen']);
const ACTIVE: ReadonlySet<RecRunStatus> = new Set(['queued', 'running']);

// ---- Titles -------------------------------------------------------------------------

/** A title's facts as a run snapshots them (`rec_items.meta`). */
interface RecTitle {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  year: number | null;
  year_end: number | null;
  seasons: number | null;
  runtime: number | null;
  vote: number | null;
  release: string | null;
}

interface Facts {
  runtime: number | null;
  vote: number | null;
  release: string | null;
}

const tv = (tmdb_id: number, title: string, year: number, seasons: number, facts: Facts & { ended?: number }): RecTitle => ({
  media_type: 'tv',
  tmdb_id,
  title,
  year,
  year_end: facts.ended ?? null,
  seasons,
  runtime: facts.runtime,
  vote: facts.vote,
  release: facts.release,
});

const movie = (tmdb_id: number, title: string, year: number, facts: Facts): RecTitle => ({
  media_type: 'movie',
  tmdb_id,
  title,
  year,
  year_end: null,
  seasons: null,
  runtime: facts.runtime,
  vote: facts.vote,
  release: facts.release,
});

/** The design's titles (js/data.js), with the TMDB ids the other mocks use. */
const TITLES = {
  severance: tv(95396, 'Severance', 2022, 2, { runtime: 50, vote: 8.4, release: '2022-02-17' }),
  frieren: tv(209867, 'Frieren: Beyond Journey’s End', 2023, 1, { runtime: 24, vote: 8.8, release: '2023-09-29' }),
  thebear: tv(136315, 'The Bear', 2022, 4, { runtime: 32, vote: 8.2, release: '2022-06-23' }),
  shogun: tv(126308, 'Shōgun', 2024, 1, { runtime: 60, vote: 8.6, release: '2024-02-27' }),
  bluey: tv(82728, 'Bluey', 2018, 3, { runtime: 7, vote: 8.7, release: '2018-10-01' }),
  pluribus: tv(222766, 'Pluribus', 2025, 1, { runtime: 55, vote: 8.0, release: '2025-11-07' }),
  goodplace: tv(66573, 'The Good Place', 2016, 4, { ended: 2020, runtime: 22, vote: 8.1, release: '2016-09-19' }),
  shrinking: tv(136311, 'Shrinking', 2023, 3, { runtime: 33, vote: 8.0, release: '2023-01-27' }),
  onlymurders: tv(107113, 'Only Murders in the Building', 2021, 5, { runtime: 33, vote: 8.0, release: '2021-08-31' }),
  mythicquest: tv(94605, 'Mythic Quest', 2020, 4, { ended: 2025, runtime: 30, vote: 7.6, release: '2020-02-07' }),
  pushingdaisies: tv(1932, 'Pushing Daisies', 2007, 2, { ended: 2009, runtime: 42, vote: 7.9, release: '2007-10-03' }),
  dandadan: tv(240411, 'Dan Da Dan', 2024, 2, { runtime: 24, vote: 8.6, release: '2024-10-04' }),
  mushishi: tv(4613, 'Mushishi', 2005, 2, { ended: 2014, runtime: 24, vote: 8.5, release: '2005-10-23' }),
  edgerunners: tv(105248, 'Cyberpunk: Edgerunners', 2022, 1, { ended: 2022, runtime: 25, vote: 8.5, release: '2022-09-13' }),
  silo: tv(125988, 'Silo', 2023, 2, { runtime: 50, vote: 8.1, release: '2023-05-04' }),
  andor: tv(83867, 'Andor', 2022, 2, { ended: 2025, runtime: 45, vote: 8.4, release: '2022-09-21' }),
  slowhorses: tv(95480, 'Slow Horses', 2022, 5, { runtime: 45, vote: 8.2, release: '2022-04-01' }),
  studio: tv(247767, 'The Studio', 2025, 1, { runtime: 30, vote: 7.9, release: '2025-03-26' }),
  neuromancer: tv(290001, 'Neuromancer', 2026, 1, { runtime: 55, vote: null, release: null }),
  havens: tv(290002, 'Avatar: Seven Havens', 2026, 1, { runtime: 24, vote: null, release: null }),
  dune2: movie(693134, 'Dune: Part Two', 2024, { runtime: 167, vote: 8.2, release: '2024-02-27' }),
  battle: movie(1054867, 'One Battle After Another', 2025, { runtime: 162, vote: 7.8, release: '2025-09-26' }),
  perfectdays: movie(976893, 'Perfect Days', 2023, { runtime: 124, vote: 7.8, release: '2023-11-10' }),
  pastlives: movie(666277, 'Past Lives', 2023, { runtime: 106, vote: 7.8, release: '2023-06-02' }),
  arrival: movie(329865, 'Arrival', 2016, { runtime: 116, vote: 7.6, release: '2016-11-10' }),
  aftersun: movie(965150, 'Aftersun', 2022, { runtime: 101, vote: 7.7, release: '2022-10-21' }),
  paprika: movie(4977, 'Paprika', 2006, { runtime: 90, vote: 7.8, release: '2006-11-25' }),
  columbus: movie(414419, 'Columbus', 2017, { runtime: 104, vote: 7.2, release: '2017-08-04' }),
  totoro: movie(8392, 'My Neighbor Totoro', 1988, { runtime: 86, vote: 8.1, release: '1988-04-16' }),
  yourname: movie(372058, 'Your Name.', 2016, { runtime: 106, vote: 8.5, release: '2016-08-26' }),
  paradiso: movie(11216, 'Cinema Paradiso', 1988, { runtime: 124, vote: 8.4, release: '1988-11-17' }),
} satisfies Record<string, RecTitle>;

export type RecTitleId = keyof typeof TITLES;

const keyOf = (mediaType: MediaType, tmdbId: number) => `${mediaType}:${tmdbId}`;
const titleKey = (id: RecTitleId) => keyOf(TITLES[id].media_type, TITLES[id].tmdb_id);
const TITLE_BY_KEY = new Map(Object.entries(TITLES).map(([id, t]) => [keyOf(t.media_type, t.tmdb_id), id as RecTitleId]));

/** A title the recommendations mock knows, by TMDB id (tests and handlers). */
export function recTitleRef(id: RecTitleId): { media_type: MediaType; tmdb_id: number } {
  return { media_type: TITLES[id].media_type, tmdb_id: TITLES[id].tmdb_id };
}

// ---- What runs write ------------------------------------------------------------------

/** A shelf item: the title, its reason (≤ 140) and an optional poster badge. */
type ShelfPick = [RecTitleId, string, string?];

interface ShelfSeed {
  slug: string;
  kind: ShelfKind;
  title: string;
  subtitle: string | null;
  items: ShelfPick[];
}

interface HeroSeed {
  id: RecTitleId;
  kicker: string;
  reason: string;
}

interface RunResult {
  shelves: ShelfSeed[];
  /** 3–9, in order: /home shows three a day. */
  hero: HeroSeed[];
}

/** The design's For You (js/data.js shelves, js/screens/home.js picks). */
const DESIGN_RESULT: RunResult = {
  shelves: [
    {
      slug: 'because-severance',
      kind: 'because',
      title: 'Because you loved Severance',
      subtitle: 'Workplace dread, slow-burn mysteries, impeccable design',
      items: [
        ['silo', 'Same sealed-world mystery, unspools one clue an episode'],
        ['andor', 'Slow, precise, and pays off everything it sets up'],
        ['pluribus', 'Vince Gilligan’s eerie new one; your top-rated genre'],
        ['slowhorses', 'Dry office politics, but spies'],
        ['mythicquest', 'Office comedy for when you need lighter'],
        ['studio', 'Long takes and workplace satire you rated highly'],
        ['goodplace', 'The same “what is this place, really?” mystery as Severance, played for laughs'],
      ],
    },
    {
      slug: 'new-this-month',
      kind: 'new',
      title: 'New this month for you',
      subtitle: 'Fresh releases matched to your taste',
      items: [
        ['pluribus', 'Premiered Nov 7; 94% of critics loved it'],
        ['studio', 'Season 2 is out; you finished season 1 in two days'],
        ['dandadan', 'Season 2 landed; high energy, high score'],
        ['aftersun', 'Just added to streaming; quiet like Perfect Days'],
        ['shrinking', 'Season 3 started; warm and funny'],
        ['battle', 'Paul Thomas Anderson’s new one, and you rated his last three highly'],
      ],
    },
    {
      slug: 'anime',
      kind: 'anime',
      title: 'Anime you’ll like',
      subtitle: 'Because of Frieren and Mushishi',
      items: [
        ['mushishi', 'The calm, episodic wonder you liked in Frieren'],
        ['dandadan', 'The opposite energy, but the same craft'],
        ['paprika', 'A 90-minute dream; Satoshi Kon at his best'],
        ['totoro', 'Ninety minutes of the warmth Frieren has between its fights'],
        ['yourname', 'Bittersweet and gorgeous, and over in one evening'],
        ['edgerunners', 'One complete season; louder, but it lands like Frieren’s farewells'],
      ],
    },
    {
      slug: 'hidden-gems',
      kind: 'gems',
      title: 'Hidden gems',
      subtitle: 'Under 50k votes, over 7.5',
      items: [
        ['columbus', 'Architecture as emotion; a sibling to Past Lives'],
        ['aftersun', 'A small film that lands enormous'],
        ['mushishi', 'Beloved and barely known outside anime'],
        ['pushingdaisies', 'Whimsical, gone too soon, complete in 2 seasons'],
        ['perfectdays', 'A quiet life in Tokyo, told with enormous warmth'],
        ['paradiso', 'A love letter to the cinema you’ve never got round to'],
        // The title page's "Why it's for you" (a shelf's reason wins over the hero's; e2e/title-movie.spec.ts)
        ['pastlives', 'Quiet, warm and under two hours.'],
      ],
    },
    {
      slug: 'coming-soon',
      kind: 'coming_soon',
      title: 'Coming soon',
      subtitle: 'On your radar, not out yet',
      items: [
        ['severance', 'Season 3 in production', '2026'],
        ['thebear', 'Season 5 confirmed', 'Summer'],
        ['frieren', 'Season 2 starts in January', 'Jan 16'],
        ['shogun', 'Season 2 is filming in Vancouver', '2026'],
        ['neuromancer', 'William Gibson, finally on screen; cyberpunk you rate highly', '2026'],
        ['havens', 'From the Avatar team; a gentle fantasy for after Frieren', '2026'],
      ],
    },
  ],
  hero: [
    { id: 'pastlives', kicker: 'Tonight’s pick', reason: 'You gave Perfect Days a 9 and finished Aftersun twice. A quiet, devastating 1 h 46 m, and it fits before midnight.' },
    { id: 'pluribus', kicker: 'New for you', reason: 'Vince Gilligan’s new series, and sci-fi with a dry sense of humour is your highest-rated corner of Trakt.' },
    { id: 'dune2', kicker: 'Ready in Infuse', reason: 'Already downloaded in 2160p Dolby Vision. You stopped Part One with 20 minutes left in 2022.' },
    { id: 'arrival', kicker: 'Something quiet', reason: 'Big ideas without a single explosion, and Villeneuve at his most precise.' },
    { id: 'studio', kicker: 'New season', reason: 'Season 2 just started, and you finished season 1 in two days.' },
    { id: 'mushishi', kicker: 'Something calm', reason: 'Self-contained folk tales, one an episode; the closest thing to Frieren’s wandering.' },
  ],
};

/** What a refresh brings: the shelves move around and a new "because" leads. */
const REFRESHED_RESULT: RunResult = {
  shelves: [
    {
      slug: 'because-the-bear',
      kind: 'because',
      title: 'Because you loved The Bear',
      subtitle: 'Kitchens, families and people at breaking point',
      items: [
        ['shrinking', 'Warm and funny about grief and work; same big heart'],
        ['onlymurders', 'A cosy puzzle with a found family at its centre'],
        ['studio', 'Workplace chaos played for laughs, in long takes'],
        ['slowhorses', 'Losers at work who turn out to be good at it'],
        ['mythicquest', 'A team that fights like a family, in 30 minutes'],
        ['goodplace', 'Kind, fast and very funny, with a mystery underneath'],
      ],
    },
    { ...DESIGN_RESULT.shelves[1], items: ['studio', 'pluribus', 'battle', 'dandadan', 'shrinking', 'aftersun'].map((id) => DESIGN_RESULT.shelves[1].items.find(([item]) => item === id)!) },
    DESIGN_RESULT.shelves[3],
    DESIGN_RESULT.shelves[2],
    DESIGN_RESULT.shelves[4],
  ],
  hero: [
    { id: 'pluribus', kicker: 'Tonight’s pick', reason: 'Vince Gilligan’s new series: strange, dry and funny, your highest-rated corner of Trakt.' },
    { id: 'studio', kicker: 'New for you', reason: 'Season 2 just started, and you finished season 1 in two days.' },
    { id: 'perfectdays', kicker: 'Something gentle', reason: 'You gave Past Lives a 9. Two quiet hours in Tokyo that leave you lighter.' },
  ],
};

// ---- Ask's answers --------------------------------------------------------------------

interface AskPick {
  id: RecTitleId;
  /** ≤ 32 characters */
  match: string;
  /** ≤ 140 characters */
  reason: string;
}

interface AskSet {
  intro: string;
  items: AskPick[];
  /** A refinement re-ranks; a fresh ask reads the history, the web and TMDB. */
  refine: boolean;
}

const ASK_SETS = {
  severance: {
    refine: false,
    intro: 'Five picks with the same “what is this place, really?” itch as Severance, but warmer and funnier. Lightest first.',
    items: [
      { id: 'goodplace', match: 'Mystery + workplace · lighter', reason: 'Severance’s “what is this place?” mystery, played for big laughs. Bright, kind and fast, with a twist every few episodes.' },
      { id: 'mythicquest', match: 'Office satire', reason: 'An office comedy about a game studio. The same absurd corporate culture, none of the dread.' },
      { id: 'onlymurders', match: 'Puzzle-box · cosy', reason: 'A cosy whodunnit with a season-long puzzle, and you rated Knives Out 9/10.' },
      { id: 'shrinking', match: 'Warm · 30-min episodes', reason: 'Warm, funny and about grief and work. You finished The Bear and Ted Lasso.' },
      { id: 'pushingdaisies', match: 'Stylised · complete', reason: 'Candy-coloured mystery with Severance-level design. Two seasons, a complete story.' },
    ],
  },
  thriller: {
    refine: false,
    intro: 'Three that fit before midnight. One is exactly 90 minutes; the others earn their extra time.',
    items: [
      { id: 'paprika', match: '90 min · dream thriller', reason: 'Exactly 90 minutes. A detective chases a stolen dream machine through Tokyo, and Satoshi Kon never lets the pace drop.' },
      { id: 'arrival', match: 'Tense · quiet', reason: 'Just under two hours and tense all the way. You gave Dune: Part Two a 9, and this is Villeneuve at his most precise.' },
      { id: 'slowhorses', match: 'Two episodes = one film', reason: 'About 45 minutes an episode; two make a tight spy-thriller evening, with the dry humour you liked in Severance.' },
    ],
  },
  anime: {
    refine: false,
    intro: 'Gentle, episodic and quietly moving, like Frieren.',
    items: [
      { id: 'mushishi', match: 'Calm · episodic wonder', reason: 'The closest thing to Frieren’s unhurried wandering: each episode is a folk tale about strange, beautiful life.' },
      { id: 'totoro', match: 'Ninety minutes of warmth', reason: 'Ninety minutes of the same warmth Frieren has between its fights.' },
      { id: 'dandadan', match: 'Same craft, more energy', reason: 'Louder and sillier, but with the attention to small character moments you rated highly in Frieren.' },
      { id: 'paprika', match: 'A 90-minute dream', reason: 'If you want one evening rather than a series: hand-drawn, dreamlike and over in 90 minutes.' },
    ],
  },
  parents: {
    refine: false,
    intro: 'Four films that work across generations: no gore, nothing awkward, and something to talk about after.',
    items: [
      { id: 'perfectdays', match: 'Gentle · moving', reason: 'A quiet life in Tokyo told with enormous warmth. It plays well to anyone who likes a slow Sunday.' },
      { id: 'pastlives', match: 'Bittersweet', reason: 'Small, adult and devastating in the best way. Subtitled in parts, mostly English.' },
      { id: 'totoro', match: 'Every age', reason: 'Ghibli is safe for every age, and it’s ninety minutes long.' },
      { id: 'arrival', match: 'Smart sci-fi', reason: 'Big ideas without a single explosion. Good for a family that argues about endings.' },
    ],
  },
  lighter: {
    refine: true,
    intro: 'Even lighter: comfort first, no dread at all.',
    items: [
      { id: 'mythicquest', match: 'Office comedy · zero dread', reason: 'All of Severance’s corporate absurdity, none of the menace. Half-hour episodes that end on a laugh.' },
      { id: 'goodplace', match: 'Mystery, played for laughs', reason: 'Still the best match: the mystery is there, but it’s kind and very funny.' },
      { id: 'bluey', match: '7-minute joy', reason: 'Grown-up enough to be the funniest thing in your library, seven minutes at a time.' },
    ],
  },
  shorter: {
    refine: true,
    intro: 'All under 30 minutes an episode, so one more is always an option.',
    items: [
      { id: 'goodplace', match: '22-min episodes', reason: 'Fast and twisty; most seasons end in under five hours.' },
      { id: 'mythicquest', match: '30-min episodes', reason: 'Tight workplace comedy, four short seasons.' },
      { id: 'studio', match: '30-min episodes', reason: 'Long takes, short episodes, and you rated Seth Rogen’s last show highly.' },
    ],
  },
  newer: {
    refine: true,
    intro: 'Everything here is from the last three years.',
    items: [
      { id: 'studio', match: '2025 · new season', reason: 'Season 2 just started. Workplace satire with Severance-level production design.' },
      { id: 'pluribus', match: '2025 · Vince Gilligan', reason: 'Strange, dry and funny. Your highest-rated corner of Trakt is exactly this.' },
      { id: 'shrinking', match: '2023 · warm', reason: 'Season 3 is airing. Warm and funny about work and grief.' },
    ],
  },
  movies: {
    refine: true,
    intro: 'One evening, no commitment.',
    items: [
      { id: 'arrival', match: 'Quiet mystery', reason: 'Severance’s unease about memory, solved in two hours, and gentler than you’d expect.' },
      { id: 'paprika', match: 'Mind-bending · 90 min', reason: 'Dreams leaking into the office. Playful rather than bleak.' },
      { id: 'totoro', match: 'Pure comfort', reason: 'As light as it gets, in 86 minutes.' },
    ],
  },
  onlyAnime: {
    refine: true,
    intro: 'Anime with the same “something is off here” hook, kept light.',
    items: [
      { id: 'dandadan', match: 'Weird · funny', reason: 'Supernatural mysteries played as a comedy. High energy, kind heart.' },
      { id: 'mushishi', match: 'Gentle mystery', reason: 'Each episode solves a small, strange mystery. Calm rather than tense.' },
      { id: 'paprika', match: 'One evening', reason: 'A 90-minute film about dreams invading work. Very Severance, much brighter.' },
    ],
  },
} satisfies Record<string, AskSet>;

/** `done.refinements`: the chips, sent back as follow-up messages. */
export const REFINEMENTS = ['Lighter', 'Shorter', 'Newer', 'Only movies', 'Only anime'];
const REFINE_SETS: Record<string, AskSet> = {
  lighter: ASK_SETS.lighter,
  shorter: ASK_SETS.shorter,
  newer: ASK_SETS.newer,
  'only movies': ASK_SETS.movies,
  'only anime': ASK_SETS.onlyAnime,
};

/** Like the design: anime or Frieren, a thriller or 90 minutes, parents or family, else Severance but lighter. */
function askSetFor(message: string, followUp: boolean): AskSet {
  const refinement = REFINE_SETS[message.trim().toLowerCase()];
  if (followUp && refinement) return refinement;
  if (/anime|frieren/i.test(message)) return ASK_SETS.anime;
  if (/thriller|90/i.test(message)) return ASK_SETS.thriller;
  if (/parent|family/i.test(message)) return ASK_SETS.parents;
  return ASK_SETS.severance;
}

const ASK_STEP_DETAIL: Record<Exclude<AskStepId, 'write'>, string> = {
  history: '2,184 plays · you loved Severance, The Bear, Ted Lasso',
  web: '14 new since Oct 1',
  tmdb: '23 candidates · 6 already seen',
  rerank: '23 checked',
};

function askSteps(set: AskSet): Array<[AskStepId, string]> {
  const reading: Array<Exclude<AskStepId, 'write'>> = set.refine ? ['rerank'] : ['history', 'web', 'tmdb'];
  return [...reading.map((step): [AskStepId, string] => [step, ASK_STEP_DETAIL[step]]), ['write', `${set.items.length} picks`]];
}

const askUsage = (set: AskSet): AskTurnUsage =>
  set.refine
    ? { input_tokens: 21_300, output_tokens: 640, web_searches: 0, cost_usd: 0.01 }
    : { input_tokens: 18_400, output_tokens: 1_150, web_searches: 2, cost_usd: 0.02 };
const askElapsed = (set: AskSet) => (set.refine ? 4_200 : 9_100);
const ASK_REQUESTS = 4;

// ---- State ------------------------------------------------------------------------------

interface MockRun {
  id: number;
  kind: RecRunKind;
  status: RecRunStatus;
  model: RecModel;
  started: number | null;
  finished: number | null;
  skipReason: RecSkipReason | null;
  error: string | null;
  /** How far it got (RUN_DONE_TICK once it succeeded). */
  tick: number;
  /** A scenario keeps an active run at this tick. */
  hold: number | null;
  fails: boolean;
  result: RunResult | null;
  /** What it costs in all. */
  cost: number;
}

interface MockTurnItem {
  id: RecTitleId;
  match_tag: string;
  reason: string;
}

interface MockTurn {
  id: number;
  message: string;
  status: AskTurnStatus;
  created: number;
  steps: AskStep[];
  intro: string;
  items: MockTurnItem[];
  refinements: string[];
  error: { code: AskErrorCode; message: string } | null;
  elapsed_ms: number | null;
  cost: number;
}

interface MockThread {
  id: number;
  title: string;
  model: RecModel;
  created: number;
  updated: number;
  turns: MockTurn[];
}

interface MockRecs {
  /** The scenario it started from: another one starts over. */
  scenario: MockScenario | null;
  /** Newest first. */
  runs: MockRun[];
  feedback: Map<string, { signal: RecSignal; source: RecFeedbackSource }>;
  threads: MockThread[];
  nextRunId: number;
  nextThreadId: number;
  nextTurnId: number;
  /** Runs started on the page fail (`recs-failed`). */
  failRuns: boolean;
  /** No picks at the start: once Trakt's first import finishes, /home queues the `first_sync` run. */
  firstSyncPending: boolean;
  /** What the page's runs and asks cost, on top of AI_SPEND. */
  spend: Record<AiFeature, { cost_usd: number; requests: number }>;
}

const at = (ms: number | null) => (ms === null ? null : new Date(ms).toISOString());
const round2 = (n: number) => Math.round(n * 100) / 100;

/** The local calendar day (the server's TZ, here the browser's), as a day count. */
const localDay = (ms: number) => {
  const d = new Date(ms);
  return Math.round(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()) / DAY);
};

function atLocal(ms: number, hours: number, dayOffset = 0): number {
  const d = new Date(ms);
  d.setDate(d.getDate() + dayOffset);
  d.setHours(hours, 0, 0, 0);
  return d.getTime();
}

/**
 * When last night's scheduled run started: today's 03:00 (local), or just
 * after midnight before then, so its picks are always today's (the hero's
 * day 0) whenever the page loads.
 */
const lastNight = (now: number) => (atLocal(now, 3) <= now - MINUTE ? atLocal(now, 3) : atLocal(now, 0));
const nextMidnight = (now: number) => atLocal(now, 0, 1);
const nextMonth = (now: number) => {
  const d = new Date(now);
  return new Date(d.getFullYear(), d.getMonth() + 1, 1).getTime();
};

/** The schedule's next run: 03:00 local, weekly on Sunday; null when manual. */
function nextRunAt(schedule: RecSchedule, now: number): number | null {
  if (schedule === 'manual') return null;
  for (let day = 0; day <= 7; day++) {
    const candidate = atLocal(now, 3, day);
    if (candidate > now && (schedule === 'nightly' || new Date(candidate).getDay() === 0)) return candidate;
  }
  return null;
}

function finishedRun(
  id: number,
  kind: RecRunKind,
  status: 'succeeded' | 'failed',
  started: number,
  seconds: number,
  model: RecModel,
  cost: number,
  result: RunResult | null
): MockRun {
  return {
    id,
    kind,
    status,
    model,
    started,
    finished: started + seconds * SECOND,
    skipReason: null,
    error: status === 'failed' ? RUN_ERROR : null,
    tick: status === 'failed' ? RUN_FAIL_TICK : RUN_DONE_TICK,
    hold: null,
    fails: status === 'failed',
    result,
    cost,
  };
}

function newRun(id: number, kind: RecRunKind, model: RecModel, result: RunResult, opts: { hold?: number; fails?: boolean } = {}): MockRun {
  return {
    id,
    kind,
    status: 'queued',
    model,
    started: null,
    finished: null,
    skipReason: null,
    error: null,
    tick: 0,
    hold: opts.hold ?? null,
    fails: opts.fails ?? false,
    result,
    cost: model === 'claude-opus-5-5' ? 0.36 : 0.07,
  };
}

/** Last night's run (the design's picks), the evening before's, a failure, the first sync, and a skip before Trakt. */
function seededRuns(now: number): MockRun[] {
  const night = lastNight(now);
  return [
    finishedRun(42, 'scheduled', 'succeeded', night, 41, 'claude-opus-5-5', 0.36, DESIGN_RESULT),
    finishedRun(41, 'manual', 'succeeded', night - 6 * HOUR, 47, 'claude-opus-5-5', 0.36, REFRESHED_RESULT),
    finishedRun(40, 'scheduled', 'failed', night - DAY, 19, 'claude-opus-5-5', 0.04, null),
    finishedRun(39, 'first_sync', 'succeeded', night - 4 * DAY - 9 * HOUR, 38, 'claude-sonnet-5-5', 0.07, DESIGN_RESULT),
    {
      ...finishedRun(38, 'scheduled', 'succeeded', night - 5 * DAY, 0, 'claude-sonnet-5-5', 0, null),
      status: 'skipped',
      started: null,
      skipReason: 'no_history',
      tick: 0,
    },
  ];
}

function completedTurn(id: number, message: string, set: AskSet, created: number): MockTurn {
  return {
    id,
    message,
    status: 'done',
    created,
    steps: askSteps(set).map(([step, detail]) => ({ step, state: 'done', detail })),
    intro: set.intro,
    items: set.items.map((item) => ({ id: item.id, match_tag: item.match, reason: item.reason })),
    refinements: [...REFINEMENTS],
    error: null,
    elapsed_ms: askElapsed(set),
    cost: askUsage(set).cost_usd,
  };
}

/** The design's conversations (js/data.js askThreads): the first is from earlier today, refined once. */
function seededThreads(now: number): MockThread[] {
  const thread = (id: number, title: string, created: number, turns: MockTurn[]): MockThread => ({
    id,
    title,
    model: 'claude-opus-5-5',
    created,
    updated: turns.at(-1)?.created ?? created,
    turns,
  });
  // Earlier today, even just after midnight: these two count towards today's asks
  const today = Math.max(now - 25 * MINUTE, atLocal(now, 0));
  const refined = today + Math.min(2 * MINUTE, (now - today) / 2);
  const yesterday = atLocal(now, 21, -1);
  return [
    thread(12, 'Something like Severance but lighter', today, [
      completedTurn(44, 'something like Severance but lighter', ASK_SETS.severance, today),
      completedTurn(45, 'Lighter', ASK_SETS.lighter, refined),
    ]),
    thread(11, 'A 90-minute thriller for tonight', yesterday, [completedTurn(43, 'a 90-minute thriller for tonight', ASK_SETS.thriller, yesterday)]),
    thread(10, 'Anime with a Frieren vibe', now - 14 * DAY, [completedTurn(42, 'anime with a Frieren vibe', ASK_SETS.anime, now - 14 * DAY)]),
    thread(9, 'Movies to watch with my parents', now - 18 * DAY, [completedTurn(41, 'Movies to watch with my parents', ASK_SETS.parents, now - 18 * DAY)]),
  ];
}

/** Scenarios where Trakt never connected (or hasn't finished its first import): no picks yet. */
const BEFORE_TRAKT: ReadonlySet<MockScenario> = new Set(['first-time', 'trakt-off', 'trakt-unconfigured', 'trakt-pending', 'trakt-denied', 'trakt-syncing']);

function startingRecs(from: MockScenario | null, now: number): MockRecs {
  const recs: MockRecs = {
    scenario: from,
    runs: seededRuns(now),
    // You liked Pluribus earlier: its cards say so
    feedback: new Map([[titleKey('pluribus'), { signal: 'like', source: 'shelf' }]]),
    threads: seededThreads(now),
    nextRunId: 43,
    nextThreadId: 13,
    nextTurnId: 46,
    failRuns: false,
    firstSyncPending: false,
    spend: { recs: { cost_usd: 0, requests: 0 }, ask: { cost_usd: 0, requests: 0 }, classifier: { cost_usd: 0, requests: 0 } },
  };
  const model = mockAiSettings().rec_model;
  switch (from) {
    case 'recs-none':
    case 'for-you-no-hero':
      return { ...recs, runs: [] };
    case 'recs-first-run':
      return { ...recs, runs: [newRun(1, 'first_sync', model, DESIGN_RESULT, { hold: RUN_HOLD_TICK })], nextRunId: 2, feedback: new Map() };
    case 'recs-refreshing':
      return { ...recs, runs: [newRun(43, 'manual', model, REFRESHED_RESULT, { hold: RUN_HOLD_TICK }), ...recs.runs], nextRunId: 44 };
    case 'recs-failed':
      return { ...recs, runs: [finishedRun(43, 'manual', 'failed', now - 20 * MINUTE, 22, model, 0.05, null), ...recs.runs], nextRunId: 44, failRuns: true };
    case 'first-time':
      return { ...recs, runs: [], threads: [], feedback: new Map(), firstSyncPending: true };
    default:
      return from !== null && BEFORE_TRAKT.has(from) ? { ...recs, runs: [], firstSyncPending: true } : recs;
  }
}

let state: MockRecs | undefined;

function current(now = Date.now()): MockRecs {
  const from = scenario();
  if (!state || state.scenario !== from) {
    state = startingRecs(from, now);
    // Each run moves along as it's read; pin the held ones where their scenario wants them
    for (const run of state.runs) while (run.hold !== null && run.tick < run.hold) step(state, run, now);
  }
  return state;
}

/** Tests: back to the starting state (it's rebuilt for the scenario on the next request). */
export function resetMockRecs(): void {
  state = undefined;
}

// ---- Runs ---------------------------------------------------------------------------------

const isActive = (run: MockRun) => ACTIVE.has(run.status);
const activeRun = (s: MockRecs) => s.runs.find(isActive);
const newestSucceeded = (s: MockRecs) => s.runs.find((run) => run.status === 'succeeded');

/** Picks need the key and `rec_enabled`. */
const picksEnabled = () => aiConfigured() && mockAiSettings().rec_enabled;

/** One read's worth of progress. */
function step(s: MockRecs, run: MockRun, now: number): void {
  if (!isActive(run) || (run.hold !== null && run.tick >= run.hold)) return;
  run.tick += 1;
  if (run.tick === 1) {
    run.status = 'running';
    run.started = now;
  }
  const end = (status: 'succeeded' | 'failed', cost: number) => {
    run.status = status;
    run.finished = now;
    s.spend.recs.cost_usd += cost;
    s.spend.recs.requests += Math.max(1, Math.round((9 * run.tick) / RUN_DONE_TICK));
  };
  if (run.fails && run.tick >= RUN_FAIL_TICK) {
    run.error = RUN_ERROR;
    end('failed', round2((run.cost * run.tick) / RUN_DONE_TICK));
  } else if (run.tick >= RUN_DONE_TICK) {
    end('succeeded', run.cost);
  }
}

/** Reads move the active run on; once Trakt's first import finishes, the first run is queued. */
function advance(s: MockRecs, now: number): void {
  const active = activeRun(s);
  if (active) {
    step(s, active, now);
    return;
  }
  if (s.firstSyncPending && traktImported('details') && picksEnabled()) {
    s.firstSyncPending = false;
    s.runs.unshift(newRun(s.nextRunId++, 'first_sync', mockAiSettings().rec_model, DESIGN_RESULT, { fails: s.failRuns }));
  }
}

const RUN_STEP_DETAIL = { history: '2,184 plays · 312 ratings', web: '14 new since Oct 1', tmdb: '148 candidates · 23 already seen' } as const;
const RUN_STEP_ORDER = ['history', 'web', 'tmdb', 'write'] as const;

/** The steps a run has reached at a tick; a finished run's are all done. */
function progressOf(run: MockRun): Pick<RunProgress, 'steps' | 'shelves_done' | 'shelves_target'> {
  if (run.status === 'skipped' || run.tick === 0) return { steps: [], shelves_done: 0, shelves_target: 0 };
  const finished = !isActive(run);
  const shelvesDone = run.status === 'succeeded' ? SHELVES_TARGET : Math.max(0, Math.min(SHELVES_TARGET - 1, (run.tick - 3) * 2));
  const reached = Math.min(run.tick, RUN_STEP_ORDER.length);
  const steps: RunStep[] = RUN_STEP_ORDER.slice(0, reached).map((name, i) => {
    const done = finished || i < reached - 1;
    const detail = name === 'write' ? `${shelvesDone} of ${SHELVES_TARGET} shelves` : done ? RUN_STEP_DETAIL[name] : null;
    return { step: name, state: done ? 'done' : 'active', detail };
  });
  return { steps, shelves_done: shelvesDone, shelves_target: SHELVES_TARGET };
}

function runCost(run: MockRun): number {
  if (run.status === 'succeeded') return run.cost;
  return round2((run.cost * Math.min(run.tick, RUN_DONE_TICK)) / RUN_DONE_TICK);
}

function summary(run: MockRun): RunSummary {
  return {
    id: run.id,
    kind: run.kind,
    status: run.status,
    model: run.model,
    started_at: at(run.started),
    finished_at: at(run.finished),
    cost_usd: runCost(run),
    skip_reason: run.skipReason,
    error: run.error,
  };
}

const progress = (run: MockRun): RunProgress => ({ ...summary(run), ...progressOf(run) });

function usageOf(run: MockRun): RecUsage {
  const share = run.status === 'skipped' ? 0 : Math.min(run.tick, RUN_DONE_TICK) / RUN_DONE_TICK;
  return {
    api_calls: Math.round(9 * share),
    input_tokens: Math.round(48_200 * share),
    output_tokens: Math.round(6_100 * share),
    cache_read_tokens: Math.round(31_400 * share),
    web_searches: run.tick >= 3 ? 6 : run.tick >= 2 ? 3 : 0,
    cost_usd: runCost(run),
  };
}

export type RecsResult<T> = { ok: true; value: T } | { ok: false; status: number; code: string; message: string; resets_at?: string };
const fail = (status: number, code: string, message: string, resetsAt?: number): RecsResult<never> => ({
  ok: false,
  status,
  code,
  message,
  ...(resetsAt === undefined ? {} : { resets_at: new Date(resetsAt).toISOString() }),
});

/** POST /recommendations/refresh: a manual run, or the active one; the checks in PLAN §6's order. */
export function refreshRecommendations(now = Date.now()): RecsResult<{ run_id: number }> {
  const s = current(now);
  if (!aiConfigured()) return fail(503, 'unavailable', 'Picks need an ANTHROPIC_API_KEY.');
  if (!mockAiSettings().rec_enabled) return fail(409, 'recs_disabled', 'Personal picks are off. Turn them on in Settings › AI & picks.');
  const active = activeRun(s);
  if (active) return { ok: true, value: { run_id: active.id } };
  if (aiUsage(now).budget_reached) return fail(429, 'budget_exceeded', 'This month’s AI budget is used up.', nextMonth(now));
  if (!traktImported('history')) return fail(409, 'no_history', 'Picks need Trakt and at least 5 titles you loved or finished.');
  const last = newestSucceeded(s);
  const run = newRun(s.nextRunId++, 'manual', mockAiSettings().rec_model, last?.result === DESIGN_RESULT ? REFRESHED_RESULT : DESIGN_RESULT, { fails: s.failRuns });
  s.runs.unshift(run);
  return { ok: true, value: { run_id: run.id } };
}

/** GET /recommendations/runs?limit= */
export function listRuns(limit: number): RunSummary[] {
  return current().runs.slice(0, limit).map(summary);
}

/** GET /recommendations/runs/{id}: reading the active run moves it on. */
export function runDetail(id: number, now = Date.now()): RunDetail | undefined {
  const s = current(now);
  const run = s.runs.find((r) => r.id === id);
  if (!run) return undefined;
  if (isActive(run)) step(s, run, now);
  return { ...progress(run), usage: usageOf(run) };
}

// ---- Cards ----------------------------------------------------------------------------------

interface ReadContext {
  library: MockLibrary;
  watchlist: Set<string>;
  feedback: MockRecs['feedback'];
}

function readContext(s: MockRecs, library: MockLibrary): ReadContext {
  const watchlist = traktImported('watchlist') ? watchlistItems(library).map((w) => keyOf(w.media_type, w.tmdb_id)) : [];
  return { library, watchlist: new Set(watchlist), feedback: s.feedback };
}

const hiddenBy = (feedback: MockRecs['feedback'], id: RecTitleId) => {
  const signal = feedback.get(titleKey(id))?.signal;
  return signal !== undefined && NEGATIVE.has(signal);
};
const likedBy = (feedback: MockRecs['feedback'], id: RecTitleId) => (feedback.get(titleKey(id))?.signal === 'like' ? 'like' : null);

/** `SimilarTitle.library` from the page's library (only catalog titles can be in it). */
function libraryOf(ctx: ReadContext, t: RecTitle): RecItem['library'] {
  if (!findTitle(t.media_type, t.tmdb_id)) return null;
  const state = buildTitle(t.media_type, t.tmdb_id, ctx.library)?.library;
  if (!state || state.library_id === null) return null;
  return {
    library_id: state.library_id,
    availability_stage: state.availability_stage,
    episodes_total: state.episodes_total,
    episodes_ready: state.episodes_ready,
  };
}

function recItem(ctx: ReadContext, id: RecTitleId, reason: string, badge: string | null = null): RecItem {
  const t = TITLES[id];
  return {
    media_type: t.media_type,
    tmdb_id: t.tmdb_id,
    title: t.title,
    year: t.year,
    year_end: t.year_end,
    poster_path: null,
    seasons: t.seasons,
    runtime_minutes: t.runtime,
    vote_average: t.vote,
    release_date: t.release,
    badge,
    reason,
    library: libraryOf(ctx, t),
    on_watchlist: ctx.watchlist.has(titleKey(id)),
    feedback: likedBy(ctx.feedback, id),
  };
}

const askItem = (ctx: ReadContext, item: MockTurnItem): AskItem => ({ ...recItem(ctx, item.id, item.reason), match_tag: item.match_tag });

function shelvesOf(ctx: ReadContext, result: RunResult): Shelf[] {
  return result.shelves
    .map((shelf) => ({
      slug: shelf.slug,
      kind: shelf.kind,
      title: shelf.title,
      subtitle: shelf.subtitle,
      items: shelf.items.filter(([id]) => !hiddenBy(ctx.feedback, id)).map(([id, reason, badge]) => recItem(ctx, id, reason, badge ?? null)),
    }))
    .filter((shelf) => shelf.items.length > 0);
}

/** Three a day: on the n-th local day since the run finished, from position 3n (wrapping), skipping hidden titles. */
function heroPicksOf(feedback: MockRecs['feedback'], run: MockRun, now: number): HeroPick[] {
  const picks = run.result?.hero ?? [];
  if (picks.length === 0) return [];
  const day = Math.max(0, localDay(now) - localDay(run.finished ?? now));
  const start = (HERO_PER_DAY * day) % picks.length;
  return [...picks.slice(start), ...picks.slice(0, start)]
    .filter((pick) => !hiddenBy(feedback, pick.id))
    .slice(0, HERO_PER_DAY)
    .map((pick) => ({ ...recTitleRef(pick.id), kicker: pick.kicker, reason: pick.reason, feedback: likedBy(feedback, pick.id) }));
}

/** GET /recommendations/home. Reading moves the active run on. */
export function recommendationsHome(library: MockLibrary, now = Date.now()): RecommendationsHome {
  const s = current(now);
  if (!picksEnabled()) return { configured: aiConfigured(), enabled: false, run: null, active_run: null, hero_picks: [], shelves: [] };
  advance(s, now);
  const run = newestSucceeded(s);
  const active = activeRun(s);
  const ctx = readContext(s, library);
  return {
    configured: true,
    enabled: true,
    run: run ? summary(run) : null,
    active_run: active ? progress(active) : null,
    hero_picks: run ? heroPicksOf(s.feedback, run, now) : [],
    shelves: run?.result ? shelvesOf(ctx, run.result) : [],
  };
}

/** The title page's `recommendation_reason`: the newest succeeded run's (a shelf's, else a hero pick's); null after negative feedback. */
export function recommendationReason(mediaType: MediaType, tmdbId: number): string | null {
  const id = TITLE_BY_KEY.get(keyOf(mediaType, tmdbId));
  if (!id || !picksEnabled()) return null;
  const s = current();
  const result = newestSucceeded(s)?.result;
  if (!result || hiddenBy(s.feedback, id)) return null;
  const onShelf = result.shelves.flatMap((shelf) => shelf.items).find(([item]) => item === id);
  return onShelf?.[1] ?? result.hero.find((pick) => pick.id === id)?.reason ?? null;
}

// ---- Feedback --------------------------------------------------------------------------------

/** POST /recommendations/feedback: an upsert. Any TMDB title takes feedback, not only the ones the mock knows. */
export function setRecFeedback(mediaType: MediaType, tmdbId: number, signal: RecSignal, source: RecFeedbackSource): void {
  current().feedback.set(keyOf(mediaType, tmdbId), { signal, source });
}

/** DELETE /recommendations/feedback/{media_type}/{tmdb_id}: idempotent. */
export function clearRecFeedback(mediaType: MediaType, tmdbId: number): void {
  current().feedback.delete(keyOf(mediaType, tmdbId));
}

/** Tests: the stored signal. */
export const recFeedback = (mediaType: MediaType, tmdbId: number) => current().feedback.get(keyOf(mediaType, tmdbId)) ?? null;

// ---- Ask -------------------------------------------------------------------------------------

const streamingTurn = (s: MockRecs) => s.threads.flatMap((t) => t.turns).find((turn) => turn.status === 'streaming');

function asksToday(s: MockRecs, now: number): number {
  if (scenario() === 'ask-limit') return ASK_DAILY_LIMIT;
  const today = localDay(now);
  return s.threads.flatMap((t) => t.turns).filter((turn) => localDay(turn.created) === today).length;
}

/**
 * A new thread's title, exactly as PLAN §6 has it: the trimmed message,
 * its first character upper-cased; over 60 characters, its first 60 cut
 * back to their last space when that space's index is over 30, trailing
 * whitespace trimmed, plus "…".
 */
function threadTitle(message: string): string {
  // Code points, as Go's runes: an emoji is one character and is never split
  const chars = Array.from(message);
  const upper = chars[0]?.toUpperCase();
  if (upper !== undefined && Array.from(upper).length === 1) chars[0] = upper; // unicode.ToUpper keeps one rune ("ß" stays)
  if (chars.length <= THREAD_TITLE_MAX) return chars.join('');
  const cut = chars.slice(0, THREAD_TITLE_MAX);
  const space = cut.lastIndexOf(' ');
  return `${(space > THREAD_TITLE_MIN_CUT ? cut.slice(0, space) : cut).join('').trimEnd()}…`;
}

function threadSummary(thread: MockThread): AskThreadSummary {
  return { id: thread.id, title: thread.title, created_at: at(thread.created)!, updated_at: at(thread.updated)!, turn_count: thread.turns.length };
}

/** GET /recommendations/threads: the newest updated first. */
export function threadSummaries(): AskThreadSummary[] {
  return [...current().threads]
    .sort((a, b) => b.updated - a.updated || b.id - a.id)
    .slice(0, THREADS_LISTED)
    .map(threadSummary);
}

function turnView(ctx: ReadContext, turn: MockTurn): AskTurn {
  const saved = turn.status !== 'streaming';
  return {
    id: turn.id,
    message: turn.message,
    status: turn.status,
    steps: saved ? turn.steps : [],
    intro: saved ? turn.intro : '',
    items: saved ? turn.items.filter((item) => !hiddenBy(ctx.feedback, item.id)).map((item) => askItem(ctx, item)) : [],
    refinements: saved ? turn.refinements : [],
    error: turn.error,
    elapsed_ms: saved ? turn.elapsed_ms : null,
    created_at: at(turn.created)!,
  };
}

/** GET /recommendations/threads/{id}: library, watchlist and feedback read now; hidden titles left out. */
export function threadView(library: MockLibrary, id: number): AskThread | undefined {
  const s = current();
  const thread = s.threads.find((t) => t.id === id);
  if (!thread) return undefined;
  const ctx = readContext(s, library);
  return { ...threadSummary(thread), model: thread.model, turns: thread.turns.map((turn) => turnView(ctx, turn)) };
}

/** A started turn: the events to stream, in order. `hold`: the stream stays open after them, until the client stops it. */
export interface AskStart {
  turn: MockTurn;
  events: AskEvent[];
  hold: boolean;
}

function askEvents(ctx: ReadContext, thread: MockThread, turn: MockTurn, set: AskSet): { events: AskEvent[]; hold: boolean } {
  const events: AskEvent[] = [{ event: 'start', data: { thread_id: thread.id, turn_id: turn.id, title: thread.title } }];
  for (const [i, [step, detail]] of askSteps(set).entries()) {
    events.push({ event: 'status', data: { step, state: 'active', detail: null } });
    // ask-hold stops at the second step (a fresh ask's web search); ask-error fails there
    if (i === 1 && scenario() === 'ask-hold') return { events, hold: true };
    // ask-working holds at TMDB with the first pick in (picks usually come after the steps; the order isn't fixed)
    if (i === 2 && scenario() === 'ask-working') {
      const [first] = set.items;
      if (first) events.push({ event: 'item', data: askItem(ctx, { id: first.id, match_tag: first.match, reason: first.reason }) });
      return { events, hold: true };
    }
    if (i === 1 && scenario() === 'ask-error') {
      events.push({ event: 'error', data: { code: 'upstream', message: 'The Anthropic API didn’t answer. Try again in a minute.' } });
      return { events, hold: false };
    }
    events.push({ event: 'status', data: { step, state: 'done', detail } });
  }
  // The intro streams a few words at a time
  const words = set.intro.split(' ');
  for (let i = 0; i < words.length; i += 4) events.push({ event: 'text', data: { delta: `${i > 0 ? ' ' : ''}${words.slice(i, i + 4).join(' ')}` } });
  for (const item of set.items) events.push({ event: 'item', data: askItem(ctx, { id: item.id, match_tag: item.match, reason: item.reason }) });
  events.push({
    event: 'done',
    data: { thread_id: thread.id, turn_id: turn.id, elapsed_ms: askElapsed(set), refinements: [...REFINEMENTS], usage: askUsage(set) },
  });
  return { events, hold: false };
}

/**
 * POST /recommendations/ask, before the stream: the checks in PLAN §6's
 * order (the body is the handler's), then a new turn (and thread) and its
 * events. A follow-up that names a refinement chip ("Lighter") re-ranks.
 */
export function startAsk(library: MockLibrary, body: { thread_id?: number; message: string }, now = Date.now()): RecsResult<AskStart> {
  const s = current(now);
  if (!aiConfigured()) return fail(503, 'unavailable', 'Ask momoshtrem needs an ANTHROPIC_API_KEY.');
  let thread = body.thread_id === undefined ? undefined : s.threads.find((t) => t.id === body.thread_id);
  if (body.thread_id !== undefined && !thread) return fail(404, 'not_found', 'That conversation doesn’t exist.');
  if (thread && thread.turns.length >= THREAD_MAX_TURNS) return fail(409, 'thread_full', 'This conversation is full. Start a new one.');
  if (streamingTurn(s)) return fail(409, 'ask_busy', 'Noir is still answering. Wait for it, or stop it.');
  if (asksToday(s, now) >= ASK_DAILY_LIMIT) return fail(429, 'ask_limit', 'That’s 30 asks today. Asks are back at midnight.', nextMidnight(now));
  if (aiUsage(now).budget_reached) return fail(429, 'budget_exceeded', 'This month’s AI budget is used up.', nextMonth(now));

  if (!thread) {
    thread = { id: s.nextThreadId++, title: threadTitle(body.message), model: mockAiSettings().rec_model, created: now, updated: now, turns: [] };
    s.threads.push(thread);
  }
  const set = askSetFor(body.message, thread.turns.length > 0);
  const turn: MockTurn = {
    id: s.nextTurnId++,
    message: body.message,
    status: 'streaming',
    created: now,
    steps: [],
    intro: '',
    items: [],
    refinements: [],
    error: null,
    elapsed_ms: null,
    cost: 0,
  };
  thread.turns.push(turn);
  thread.updated = now;
  const { events, hold } = askEvents(readContext(s, library), thread, turn, set);
  return { ok: true, value: { turn, events, hold } };
}

/** The handler sent an event: the turn keeps what it showed, saved for the thread when it ends. */
export function recordAskEvent(turn: MockTurn, event: AskEvent): void {
  switch (event.event) {
    case 'status':
      turn.steps = applyStatus(turn.steps, event.data);
      break;
    case 'text':
      turn.intro += event.data.delta;
      break;
    case 'item': {
      const id = TITLE_BY_KEY.get(keyOf(event.data.media_type, event.data.tmdb_id));
      if (id) turn.items.push({ id, match_tag: event.data.match_tag, reason: event.data.reason });
      break;
    }
    case 'done':
      turn.refinements = event.data.refinements;
      turn.elapsed_ms = event.data.elapsed_ms;
      turn.cost = event.data.usage.cost_usd;
      break;
    case 'error':
      turn.error = event.data;
      break;
  }
}

/** The turn ended (done, an error event, or the client went away): the usage spent so far is recorded. Idempotent. */
export function endAskTurn(turn: MockTurn, status: Exclude<AskTurnStatus, 'streaming'>, now = Date.now()): void {
  if (turn.status !== 'streaming') return;
  const s = current(now);
  turn.status = status;
  turn.elapsed_ms ??= now - turn.created;
  // A turn cut short still paid for what it read
  if (status !== 'done' && turn.steps.length > 0) turn.cost = 0.01;
  // An ended turn never shows a spinner: the step still active when it failed or stopped is dropped (PLAN §6)
  turn.steps = turn.steps.filter((step) => step.state !== 'active');
  s.spend.ask.cost_usd += turn.cost;
  s.spend.ask.requests += status === 'done' ? ASK_REQUESTS : 1;
  const thread = s.threads.find((t) => t.turns.includes(turn));
  if (thread) thread.updated = now;
}

// ---- Usage ------------------------------------------------------------------------------------

const FEATURES: readonly AiFeature[] = ['recs', 'ask', 'classifier'];
const mean = (values: number[]) => (values.length === 0 ? null : round2(values.reduce((a, b) => a + b, 0) / values.length));

/** GET /settings/ai/usage: this month (local), the page's runs and asks included. */
export function aiUsage(now = Date.now()): AiUsage {
  const s = current(now);
  const settings = mockAiSettings();
  const byFeature = FEATURES.map((feature) => ({
    feature,
    cost_usd: round2(AI_SPEND[feature].cost_usd + s.spend[feature].cost_usd),
    requests: AI_SPEND[feature].requests + s.spend[feature].requests,
  }));
  let total = round2(byFeature.reduce((sum, row) => sum + row.cost_usd, 0));
  const budget = settings.monthly_budget_usd;
  // ask-budget: the budget is all spent, picks took the rest
  if (scenario() === 'ask-budget' && budget !== null && total < budget) {
    byFeature[0].cost_usd = round2(byFeature[0].cost_usd + budget - total);
    total = budget;
  }
  const d = new Date(now);
  const succeeded = (model: RecModel) => s.runs.filter((run) => run.status === 'succeeded' && run.model === model).slice(0, 5).map((run) => run.cost);
  const doneTurns = s.threads
    .flatMap((t) => t.turns)
    .filter((turn) => turn.status === 'done')
    .sort((a, b) => b.created - a.created)
    .slice(0, 20);
  const lastFinished = s.runs.find((run) => !isActive(run));
  const active = activeRun(s);
  const scheduled = aiConfigured() && settings.rec_enabled ? nextRunAt(settings.rec_schedule, now) : null;
  return {
    configured: aiConfigured(),
    month: `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`,
    resets_at: at(nextMonth(now))!,
    total_usd: total,
    budget_usd: budget,
    budget_reached: budget !== null && total >= budget,
    by_feature: byFeature,
    recs: { last_run: lastFinished ? summary(lastFinished) : null, active_run: active ? progress(active) : null, next_run_at: at(scheduled) },
    estimates: {
      run_usd: { 'claude-opus-5-5': mean(succeeded('claude-opus-5-5')), 'claude-sonnet-5-5': mean(succeeded('claude-sonnet-5-5')) },
      ask_usd: mean(doneTurns.map((turn) => turn.cost)),
    },
    asks_today: asksToday(s, now),
    ask_daily_limit: ASK_DAILY_LIMIT,
  };
}

/** For GET /status: every AI feature's spend this month. */
export const aiMonthToDate = (): number => aiUsage().total_usd;
