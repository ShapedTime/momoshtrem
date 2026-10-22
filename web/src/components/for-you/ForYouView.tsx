'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { RefreshCw } from 'lucide-react';
import { Banner, Button, ButtonLink, EmptyState, LoadingRegion, PosterSkeletons, Skeleton, WideSkeletons } from '@/components/ui';
import { useAskAvailable } from '@/hooks/useAskAvailable';
import { useTraktIntegration } from '@/hooks/useTraktIntegration';
import type { RunProgress, Shelf, TraktIntegration } from '@/lib/api/contract';
import {
  discoverPagesQuery,
  libraryQuery,
  newEpisodesQuery,
  recKeys,
  recommendationsHomeQuery,
  recRunsQuery,
  titleKeys,
  TRAKT_POLL_MS,
  trendingTitlesQuery,
  upNextQuery,
  watchlistQuery,
} from '@/lib/api/endpoints';
import { DEFAULT_FILTERS, type DiscoverFilters } from '@/lib/discover/filters';
import { newEpisodeCard, upNextCard } from '@/lib/for-you/episode-card';
import { forYouHeroPicks, heroPicks, localDateKey } from '@/lib/for-you/hero';
import { failedRefresh } from '@/lib/for-you/picks';
import { askSuggestions, layoutShelves, refreshingShelfSlug } from '@/lib/for-you/shelves';
import { firstPicksRun, forYouState } from '@/lib/for-you/state';
import { REAUTH_TITLE, reauthBody } from '@/lib/settings/trakt';
import { plural } from '@/lib/title/format';
import { FirstSync } from './FirstSync';
import { HomeHero, HomeHeroSkeleton } from './HomeHero';
import { AskBand, AskEntryPhone, PicksError, PicksFoot, PicksLoading, PicksNote, RefreshBanner, ShelfRail, WritingShelf } from './Picks';
import { EpisodeRail, railQuery, TitleRail } from './Rails';
import { ConnectCard, TRAKT_SETTINGS_HREF, Welcome } from './Welcome';

/** Discover's presets for the rails without Trakt (sorted by popularity). */
const POPULAR_ANIME: DiscoverFilters = { ...DEFAULT_FILTERS, type: 'tv', anime: true };
const POPULAR_MOVIES: DiscoverFilters = { ...DEFAULT_FILTERS, type: 'movie' };

/** Today, read once per visit: the labels ("Today", "Airs Sun") and the hero's day. */
function useToday(): Date {
  const [today] = useState(() => new Date());
  return today;
}

function useTrendingRail() {
  const trending = useQuery(trendingTitlesQuery());
  return { trending, rail: railQuery(trending, (titles) => titles) };
}

/** The For You page title for screen readers: the hero's title is a pick, not the page. */
function PageTitle() {
  return <h1 className="sr-only">For You</h1>;
}

/** Before For You knows which page to show: the hero's box and two rails, at their final sizes, after 150 ms. */
function ForYouLoading() {
  return (
    <>
      <PageTitle />
      <HomeHeroSkeleton />
      <LoadingRegion label="Loading For You" className="page page-sections home-body">
        <div className="section">
          <Skeleton shape="line" width={160} height={20} />
          <div className="rail rail--wide overflow-hidden">
            <WideSkeletons count={4} />
          </div>
        </div>
        <div className="section">
          <Skeleton shape="line" width={220} height={20} />
          <div className="rail rail--posters overflow-hidden">
            <PosterSkeletons count={8} />
          </div>
        </div>
      </LoadingRegion>
    </>
  );
}

/** No Trakt and an empty library. */
function FirstTimeHome() {
  const { rail } = useTrendingRail();
  return (
    <div className="page-sections welcome-page">
      <Welcome />
      <div className="page">
        <TitleRail title="Trending this week" subtitle="From TMDB, while I get to know you" query={rail} errorTitle="Couldn’t load trending titles" library={false} />
      </div>
    </div>
  );
}

/** Without Trakt: a trending hero, what connecting brings, new episodes of library shows, then trending. */
function TraktOffHome({ traktFailed, onRetryTrakt }: { traktFailed: boolean; onRetryTrakt: () => void }) {
  const today = useToday();
  const { trending, rail: trendingRail } = useTrendingRail();
  const newEpisodes = useQuery(newEpisodesQuery());
  const anime = useInfiniteQuery(discoverPagesQuery(POPULAR_ANIME));
  const movies = useInfiniteQuery(discoverPagesQuery(POPULAR_MOVIES));
  const picks = useMemo(() => heroPicks({ upNext: [], watchlist: [], trending: trending.data ?? [], date: localDateKey(today) }), [trending.data, today]);

  return (
    <>
      <PageTitle />
      {trending.isPending ? <HomeHeroSkeleton /> : picks.length > 0 && <HomeHero picks={picks} />}
      <div className="page page-sections home-body">
        {traktFailed ? (
          <Banner
            tone="error"
            title="Couldn’t check Trakt"
            action={
              <Button size="sm" onClick={onRetryTrakt}>
                <RefreshCw size={16} aria-hidden="true" /> Try again
              </Button>
            }
          >
            momoshtrem didn’t say whether Trakt is connected. Trending still works.
          </Banner>
        ) : (
          <ConnectCard />
        )}
        <EpisodeRail
          title="New episodes"
          subtitle="From your library"
          query={railQuery(newEpisodes, (data) => data.items.map((item) => newEpisodeCard(item, today)))}
          errorTitle="Couldn’t load new episodes"
        />
        <TitleRail title="Trending this week" query={trendingRail} errorTitle="Couldn’t load trending titles" />
        <TitleRail title="Popular anime" query={railQuery(anime, (data) => data.pages[0]?.items ?? [])} errorTitle="Couldn’t load popular anime" />
        <TitleRail title="Popular movies" query={railQuery(movies, (data) => data.pages[0]?.items ?? [])} errorTitle="Couldn’t load popular movies" />
      </div>
    </>
  );
}

/** The first import is running (or stopped): its steps, and Up next as it fills. Then Noir writes the first shelves. */
function FirstSyncHome({ trakt, firstRun }: { trakt: TraktIntegration; firstRun: RunProgress | null }) {
  const today = useToday();
  const syncing = trakt.sync.status === 'syncing';
  const upNext = useQuery({ ...upNextQuery(), refetchInterval: syncing ? TRAKT_POLL_MS : false, refetchIntervalInBackground: false });
  const cards = railQuery(upNext, (data) => data.items.map((item) => upNextCard(item, today)));
  return (
    <div className="page-sections first-sync-page">
      <PageTitle />
      <FirstSync trakt={trakt} firstRun={firstRun} />
      {(syncing || (cards.items?.length ?? 0) > 0) && (
        <div className="page">
          <EpisodeRail
            title="Up next"
            subtitle={cards.items?.length ? 'From your Trakt progress' : 'Finding your next episodes…'}
            query={cards}
            errorTitle="Couldn’t load Up next"
            filling={syncing}
          />
        </div>
      )}
      {firstRun && (
        <div className="page">
          <WritingShelf title="Your picks" subtitle="Writing…" />
        </div>
      )}
    </div>
  );
}

/** Up next with nothing in it while other rows have something. */
function CaughtUp() {
  return (
    <p className="rail-note">
      You’re caught up. When Trakt sees you start a show, its next episode shows up here.
    </p>
  );
}

/**
 * When a run ends, the runs list (the failed-refresh note) and the title
 * pages' "Why it's for you" are stale, and /home is read once more for the
 * new picks.
 */
function useRunEnd(activeRunId: number | null) {
  const queryClient = useQueryClient();
  const previous = useRef(activeRunId);
  useEffect(() => {
    const was = previous.current;
    previous.current = activeRunId;
    if (was === null || was === activeRunId) return;
    void queryClient.invalidateQueries({ queryKey: recKeys.home });
    void queryClient.invalidateQueries({ queryKey: [...recKeys.all, 'runs'] });
    void queryClient.invalidateQueries({ queryKey: titleKeys.all });
  }, [activeRunId, queryClient]);
}

/**
 * Connected (HANDOFF §4.3; home.js): the hero (Noir's Tonight's pick, else
 * For You's own), Up next, New episodes, Noir's first shelf, the Ask band,
 * the other shelves, the watchlist, Coming soon, and the picks footer.
 * Without picks (none yet, switched off, no key) trending fills in.
 */
function PersonalHome({ trakt }: { trakt: TraktIntegration }) {
  const today = useToday();
  const showWatchlist = trakt.settings.show_watchlist;
  const upNext = useQuery(upNextQuery());
  const newEpisodes = useQuery(newEpisodesQuery());
  const watchlist = useQuery({ ...watchlistQuery(), enabled: showWatchlist });
  const { trending, rail: trendingRail } = useTrendingRail();
  const recs = useQuery(recommendationsHomeQuery());
  const askAvailable = useAskAvailable();

  // Picks switched off (or no key) read as no picks: trending fills in
  const home = recs.data?.enabled ? recs.data : undefined;
  const run = home?.run ?? null;
  const activeRun = home?.active_run ?? null;
  const shelves = useMemo(() => layoutShelves(run && home ? home.shelves : []), [run, home]);
  const hasShelves = shelves.lead !== null || shelves.comingSoon.length > 0;
  const refreshing = activeRun && hasShelves ? refreshingShelfSlug(shelves) : null;
  const latestRun = useQuery({ ...recRunsQuery(1), enabled: home !== undefined });
  const failed = activeRun ? null : failedRefresh(latestRun.data?.items[0], run);
  const picksFailed = recs.isError && !recs.data;
  useRunEnd(activeRun?.id ?? null);

  const upNextRail = railQuery(upNext, (data) => data.items.map((item) => upNextCard(item, today)));
  const newRail = railQuery(newEpisodes, (data) => data.items.map((item) => newEpisodeCard(item, today)));
  const watchlistRail = railQuery(watchlist, (data) => data.items, showWatchlist);

  // The hero waits for what it picks from, so it never swaps a trending pick for an Up next one,
  // or its own pick for Noir's, mid-view
  const recommended = useMemo(() => home?.hero_picks ?? [], [home]);
  const ownSettled = !upNext.isPending && (!showWatchlist || !watchlist.isPending) && !trending.isPending;
  const settled = !recs.isPending && (recommended.length > 0 || ownSettled);
  const picks = useMemo(
    () =>
      forYouHeroPicks(recommended, {
        upNext: upNext.data?.items ?? [],
        watchlist: showWatchlist ? (watchlist.data?.items ?? []) : [],
        trending: trending.data ?? [],
        date: localDateKey(today),
      }),
    [recommended, upNext.data, watchlist.data, trending.data, showWatchlist, today]
  );
  const suggestions = useMemo(() => askSuggestions(upNext.data?.items.find((item) => item.source === 'progress')?.show.title), [upNext.data]);

  const rails = [upNextRail, newRail, watchlistRail];
  const allFailed = rails.every((rail, i) => rail.failed || (i === 2 && !showWatchlist));
  const nothing = rails.every((rail) => rail.items !== undefined && rail.items.length === 0);
  const retryAll = () => rails.forEach((rail) => rail.failed && rail.retry());

  const relinking = trakt.state === 'reauth' || trakt.state === 'pending';
  const watchlistCount = trakt.counts.watchlist || watchlistRail.items?.length || 0;
  const shelfRail = (shelf: Shelf) =>
    shelf.slug === refreshing ? <WritingShelf key={shelf.slug} title={shelf.title} subtitle="Refreshing…" /> : <ShelfRail key={shelf.slug} shelf={shelf} today={today} />;

  return (
    <>
      <PageTitle />
      {!settled ? <HomeHeroSkeleton /> : picks.length > 0 && <HomeHero picks={picks} upNext={upNext.data?.items} />}
      <div className="page page-sections home-body">
        {relinking && (
          <Banner
            tone="warning"
            title={trakt.state === 'pending' ? 'Finish connecting Trakt' : REAUTH_TITLE}
            action={
              <ButtonLink href={TRAKT_SETTINGS_HREF} size="sm">
                {trakt.state === 'pending' ? 'Open Settings' : 'Reconnect'}
              </ButtonLink>
            }
          >
            {reauthBody(trakt)}
          </Banner>
        )}
        {activeRun && <RefreshBanner run={activeRun} first={run === null} />}
        {failed && <PicksNote run={failed} first={run === null} />}
        {askAvailable && <AskEntryPhone />}
        {allFailed ? (
          <Banner
            tone="error"
            title="Couldn’t load your Trakt rows"
            action={
              <Button size="sm" onClick={retryAll} loading={rails.some((rail) => rail.retrying)}>
                Try again
              </Button>
            }
          >
            Up next, new episodes and the watchlist didn’t load. The rest of the page still works.
          </Banner>
        ) : nothing ? (
          <EmptyState
            cat="sleep"
            title="Nothing up next yet"
            headingLevel="h2"
            actions={
              <ButtonLink href="/discover" variant="secondary">
                Browse Discover
              </ButtonLink>
            }
          >
            Trakt has no shows in progress and nothing new aired this week. Start something and this page fills in.
          </EmptyState>
        ) : (
          <>
            <EpisodeRail title="Up next" subtitle="From your Trakt progress" query={upNextRail} errorTitle="Couldn’t load Up next" empty={<CaughtUp />} />
            <EpisodeRail title="New episodes" subtitle="For shows you watch" query={newRail} errorTitle="Couldn’t load new episodes" />
          </>
        )}
        {recs.isPending ? (
          <PicksLoading />
        ) : picksFailed ? (
          <PicksError onRetry={() => void recs.refetch()} retrying={recs.isRefetching} />
        ) : (
          shelves.lead && shelfRail(shelves.lead)
        )}
        {activeRun && run === null && <WritingShelf title="Your picks" subtitle="Writing…" />}
        {askAvailable && <AskBand suggestions={suggestions} />}
        {shelves.rest.map(shelfRail)}
        {showWatchlist && !allFailed && !nothing && (
          <TitleRail
            title="Your Trakt watchlist"
            subtitle={watchlistCount > 0 ? `${plural(watchlistCount, 'title')} · ${trakt.counts.watchlist_in_library} in your library` : undefined}
            query={watchlistRail}
            errorTitle="Couldn’t load your watchlist"
            errorDetail="momoshtrem didn’t answer. The rest of the page still works."
          />
        )}
        {shelves.comingSoon.map(shelfRail)}
        {!hasShelves && !recs.isPending && <TitleRail title="Trending this week" query={trendingRail} errorTitle="Couldn’t load trending titles" />}
        {run && hasShelves && <PicksFoot run={run} now={today} />}
      </div>
    </>
  );
}

/**
 * When Trakt's first import ends, the backend queues Noir's first run: read
 * /home again, and stay on the first sync until it answers, so the personal
 * page doesn't flash up before "Writing your shelves".
 */
function useAfterFirstImport(importing: boolean, recs: { dataUpdatedAt: number; errorUpdatedAt: number }): boolean {
  const queryClient = useQueryClient();
  const readAt = Math.max(recs.dataUpdatedAt, recs.errorUpdatedAt);
  const [wasImporting, setWasImporting] = useState(importing);
  const [readAtEnd, setReadAtEnd] = useState<number | null>(null);
  if (wasImporting !== importing) {
    setWasImporting(importing);
    setReadAtEnd(wasImporting && !importing ? readAt : null);
  }
  useEffect(() => {
    if (readAtEnd !== null) void queryClient.invalidateQueries({ queryKey: recKeys.home });
  }, [readAtEnd, queryClient]);
  return readAtEnd !== null && readAt === readAtEnd;
}

/**
 * For You (`/`, PLAN F5; #135, #155): "What should I watch tonight, and is
 * it ready?" Which page it shows follows lib/for-you/state.ts: loading, first
 * time, Trakt off, the first sync (then Noir's first shelves), or the
 * personal page with Noir's picks.
 */
export function ForYouView() {
  const integration = useTraktIntegration();
  const library = useQuery(libraryQuery({ page_size: 1 }));
  const recs = useQuery(recommendationsHomeQuery());
  // A failed poll keeps what was known; only a first load that fails reads as "no Trakt"
  const trakt = integration.data ?? (integration.isError ? null : undefined);
  const importing = trakt?.sync.kind === 'initial' && trakt.sync.status === 'syncing';
  const checkingPicks = useAfterFirstImport(importing, recs);
  const firstRun = firstPicksRun(recs.data);
  const state = forYouState({
    trakt,
    libraryTotal: library.data?.summary.total ?? (library.isError ? null : undefined),
    writingFirstPicks: firstRun !== null || checkingPicks,
    picksPending: recs.isPending,
  });

  switch (state) {
    case 'loading':
      return <ForYouLoading />;
    case 'first_time':
      return <FirstTimeHome />;
    case 'trakt_off':
      return <TraktOffHome traktFailed={trakt === null} onRetryTrakt={() => void integration.refetch()} />;
    case 'first_sync':
      return trakt ? <FirstSyncHome trakt={trakt} firstRun={firstRun} /> : null;
    case 'default':
      return trakt ? <PersonalHome trakt={trakt} /> : null;
  }
}
