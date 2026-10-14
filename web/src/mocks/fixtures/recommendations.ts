import type { AskThread, AskThreadSummary, RecItem, RecRun, RecommendationsHome } from '@/lib/api/contract';
import { CATALOG, iso } from './catalog';

const items: RecItem[] = CATALOG.map((entry) => ({
  media_type: entry.media_type,
  tmdb_id: entry.tmdb_id,
  title: entry.title,
  year: entry.year,
  poster_path: null,
  reason: `Because you loved Mushishi: ${entry.genres.join(', ').toLowerCase()}.`,
  in_library: entry.tmdb_id % 2 === 0,
  on_watchlist: false,
}));

export const REC_HOME: RecommendationsHome = {
  run: { id: 42, finished_at: iso(0) },
  hero_picks: [
    { tmdb_id: 95396, media_type: 'tv', kicker: 'Tonight’s pick', reason: 'Season 2 finished downloading, and you rated season 1 a 10.' },
    { tmdb_id: 666277, media_type: 'movie', kicker: 'Something gentle', reason: 'Quiet, warm and under two hours.' },
  ],
  shelves: [
    { slug: 'because-severance', title: 'Because you loved Severance', subtitle: 'Slow-burn mysteries with a big idea', items },
    { slug: 'new-this-month', title: 'New this month for you', subtitle: null, items: [...items].reverse() },
    { slug: 'anime', title: 'Anime you’ll like', subtitle: 'Picked from your Frieren and Mushishi history', items: items.slice(0, 4) },
  ],
};

export const REC_RUN: RecRun = {
  id: 42,
  kind: 'nightly',
  status: 'succeeded',
  model: 'claude-opus-5-5',
  progress: [
    { step: 'Reading your history', detail: '1,284 plays' },
    { step: 'Finding candidates', detail: '148 titles' },
    { step: 'Writing your picks', detail: '3 of 3 shelves' },
  ],
  usage: { input_tokens: 48_200, output_tokens: 6_100, web_searches: 6 },
  cost_usd: 0.74,
  error: null,
  started_at: iso(0),
  finished_at: iso(0),
};

export const THREADS: AskThreadSummary[] = [
  { id: 7, title: 'Something like Severance but lighter', created_at: iso(1), updated_at: iso(1) },
  { id: 6, title: 'Anime movies under 2 hours', created_at: iso(5), updated_at: iso(5) },
];

export function thread(id: number): AskThread {
  const summary = THREADS.find((t) => t.id === id) ?? THREADS[0];
  return {
    ...summary,
    messages: [
      { role: 'user', text: summary.title, items: [], created_at: summary.created_at },
      { role: 'assistant', text: 'Here are three that keep the mystery but lose the dread.', items: items.slice(1, 4), created_at: summary.created_at },
    ],
  };
}

/** The events Ask streams, in order. */
export const ASK_EVENTS = [
  { event: 'status', data: { step: 'Checking your history…', detail: null } },
  { event: 'status', data: { step: 'Searching the web', detail: 'new comedies 2026' } },
  { event: 'item', data: { ...items[3], match_tag: 'Workplace satire, lighter' } },
  { event: 'text', data: { delta: 'Try The Studio: ' } },
  { event: 'text', data: { delta: 'same deadpan workplace absurdity, none of the dread.' } },
  { event: 'done', data: { thread_id: 8, usage: { input_tokens: 9_800, output_tokens: 420 }, refinements: ['lighter', 'shorter', 'only anime'] } },
] as const;
