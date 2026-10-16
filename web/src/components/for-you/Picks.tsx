'use client';

import Link from 'next/link';
import { TriangleAlert } from 'lucide-react';
import { AskEntry } from '@/components/ask/AskEntry';
import { CatAvatar, CatEyes } from '@/components/brand';
import { RecPosterCard } from '@/components/recommendations';
import { LoadingRegion, PosterSkeletons, Rail, RailItem, Skeleton } from '@/components/ui';
import type { RunProgress, RunSummary, Shelf } from '@/lib/api/contract';
import { picksFootLine, refreshSummary } from '@/lib/for-you/picks';
import { comingSoonBadge } from '@/lib/for-you/shelves';
import { askHref } from '@/lib/recommendations/ask-href';
import { RailError, RailFrame } from './Rails';

/**
 * Noir's picks on For You (#155; home.js): the shelves, the refreshing
 * banner, the Ask entries and the picks footer.
 */

/** Settings › AI & picks (#159): the schedule lives there. */
export const PICKS_SETTINGS_HREF = '/settings?section=ai';

/** One of Noir's shelves: a reason and feedback on every card; Coming soon cards carry their date. */
export function ShelfRail({ shelf, today }: { shelf: Shelf; today: Date }) {
  const comingSoon = shelf.kind === 'coming_soon';
  return (
    <Rail title={shelf.title} subtitle={shelf.subtitle ?? undefined}>
      {shelf.items.map((item) => (
        <RailItem key={`${item.media_type}:${item.tmdb_id}`}>
          <RecPosterCard item={item} source="shelf" comingSoon={comingSoon ? comingSoonBadge(item, today) : null} />
        </RailItem>
      ))}
    </Rail>
  );
}

/** A shelf being rewritten by the run under way: its heading over skeletons (home.js: "Refreshing…"). */
export function WritingShelf({ title, subtitle }: { title: string; subtitle: string }) {
  return (
    <RailFrame title={title} subtitle={subtitle}>
      <div className="rail rail--posters overflow-hidden" aria-hidden="true">
        <PosterSkeletons count={8} />
      </div>
    </RailFrame>
  );
}

/** Before /recommendations/home answers: one shelf's worth of skeletons, after 150 ms. */
export function PicksLoading() {
  return (
    <LoadingRegion label="Loading your picks" className="section">
      <Skeleton shape="line" width={240} height={20} />
      <div className="rail rail--posters overflow-hidden">
        <PosterSkeletons count={8} />
      </div>
    </LoadingRegion>
  );
}

/** /recommendations/home didn't answer: the rest of the page (and trending) carries on. */
export function PicksError({ onRetry, retrying }: { onRetry: () => void; retrying: boolean }) {
  return (
    <RailFrame title="Your picks">
      <RailError title="Couldn’t load your picks" onRetry={onRetry} retrying={retrying}>
        momoshtrem didn’t answer. Trending fills in for now.
      </RailError>
    </RailFrame>
  );
}

/**
 * While a run is queued or running (HANDOFF §4.3: named steps): Noir's eyes,
 * the step under way and the shelves written so far. The old picks stay
 * until it's done; "You can keep browsing" on wider screens.
 */
export function RefreshBanner({ run, first }: { run: RunProgress; first: boolean }) {
  return (
    <div className="refresh-banner glass" role="status">
      <span className="eyes-sm" aria-hidden="true">
        <CatEyes width={40} />
      </span>
      <div className="grow">
        <strong>{first ? 'Writing your first picks' : 'Refreshing your picks'}</strong>
        <span className="t-small text-fg-2">{refreshSummary(run)}</span>
      </div>
      <span className="t-small text-fg-3 hidden shrink-0 sm:inline">You can keep browsing</span>
    </div>
  );
}

/** The last refresh failed: the shelves below are the earlier ones. Quiet, since nothing's lost. */
export function PicksNote({ run }: { run: RunSummary }) {
  return (
    <p className="picks-note t-small">
      <TriangleAlert size={16} aria-hidden="true" />
      <span>
        <strong className="font-medium text-fg-1">Couldn’t refresh your picks.</strong> {run.error ?? 'These are from the last run that finished.'}
      </span>
    </p>
  );
}

/** "Picks written by Noir at 03:00 from your Trakt history. Change schedule" */
export function PicksFoot({ run, now }: { run: RunSummary; now: Date }) {
  return (
    <p className="picks-foot t-small text-fg-3">
      <CatAvatar size={20} />
      <span>{picksFootLine(run, now)}</span>
      <Link className="text-fg-2" href={PICKS_SETTINGS_HREF}>
        Change schedule
      </Link>
    </p>
  );
}

const ASK_TITLE = 'Ask momoshtrem…';
const ASK_EXAMPLE = '“a 90-minute thriller for tonight”';

/** The phone's way into Ask, under the hero (`lg` and up has the band instead). */
export function AskEntryPhone() {
  return (
    <div className="lg:hidden">
      <AskEntry href={askHref()} title={ASK_TITLE} sub={ASK_EXAMPLE} avatarSize={40} />
    </div>
  );
}

/** Desktop's Ask band after the first shelf: the entry and three suggestions, each starting an ask. */
export function AskBand({ suggestions }: { suggestions: string[] }) {
  return (
    <section className="ask-band hidden lg:grid" aria-label="Ask momoshtrem">
      <AskEntry href={askHref()} title={ASK_TITLE} sub={ASK_EXAMPLE} avatarSize={40} />
      <ul className="ask-suggest" aria-label="Try asking">
        {suggestions.map((q) => (
          <li key={q}>
            <Link className="fchip" href={askHref(q)}>
              {q}
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
