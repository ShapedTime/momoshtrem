'use client';

import { useMemo, useState } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { RefreshCw } from 'lucide-react';
import { Banner, Button, ButtonLink, EmptyState, LoadingRegion, PosterSkeletons, Skeleton, WideSkeletons } from '@/components/ui';
import { useTraktIntegration } from '@/hooks/useTraktIntegration';
import type { TraktIntegration } from '@/lib/api/contract';
import {
  discoverPagesQuery,
  libraryQuery,
  newEpisodesQuery,
  TRAKT_POLL_MS,
  trendingTitlesQuery,
  upNextQuery,
  watchlistQuery,
} from '@/lib/api/endpoints';
import { DEFAULT_FILTERS, type DiscoverFilters } from '@/lib/discover/filters';
import { newEpisodeCard, upNextCard } from '@/lib/for-you/episode-card';
import { heroPicks, localDateKey } from '@/lib/for-you/hero';
import { forYouState } from '@/lib/for-you/state';
import { REAUTH_TITLE, reauthBody } from '@/lib/settings/trakt';
import { plural } from '@/lib/title/format';
import { FirstSync } from './FirstSync';
import { HomeHero, HomeHeroSkeleton } from './HomeHero';
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
function ForYouLoading({ username }: { username: string }) {
  return (
    <>
      <PageTitle />
      <HomeHeroSkeleton username={username} />
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
function TraktOffHome({ username, traktFailed, onRetryTrakt }: { username: string; traktFailed: boolean; onRetryTrakt: () => void }) {
  const today = useToday();
  const { trending, rail: trendingRail } = useTrendingRail();
  const newEpisodes = useQuery(newEpisodesQuery());
  const anime = useInfiniteQuery(discoverPagesQuery(POPULAR_ANIME));
  const movies = useInfiniteQuery(discoverPagesQuery(POPULAR_MOVIES));
  const picks = useMemo(() => heroPicks({ upNext: [], watchlist: [], trending: trending.data ?? [], date: localDateKey(today) }), [trending.data, today]);

  return (
    <>
      <PageTitle />
      {trending.isPending ? <HomeHeroSkeleton username={username} /> : picks.length > 0 && <HomeHero username={username} picks={picks} />}
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

/** The first import is running (or stopped): its steps, and Up next as it fills. */
function FirstSyncHome({ trakt }: { trakt: TraktIntegration }) {
  const today = useToday();
  const syncing = trakt.sync.status === 'syncing';
  const upNext = useQuery({ ...upNextQuery(), refetchInterval: syncing ? TRAKT_POLL_MS : false, refetchIntervalInBackground: false });
  const cards = railQuery(upNext, (data) => data.items.map((item) => upNextCard(item, today)));
  return (
    <div className="page-sections first-sync-page">
      <PageTitle />
      <FirstSync trakt={trakt} />
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

/** Connected: the hero, Up next, New episodes and the watchlist, then trending until #9's shelves. */
function PersonalHome({ username, trakt }: { username: string; trakt: TraktIntegration }) {
  const today = useToday();
  const showWatchlist = trakt.settings.show_watchlist;
  const upNext = useQuery(upNextQuery());
  const newEpisodes = useQuery(newEpisodesQuery());
  const watchlist = useQuery({ ...watchlistQuery(), enabled: showWatchlist });
  const { trending, rail: trendingRail } = useTrendingRail();

  const upNextRail = railQuery(upNext, (data) => data.items.map((item) => upNextCard(item, today)));
  const newRail = railQuery(newEpisodes, (data) => data.items.map((item) => newEpisodeCard(item, today)));
  const watchlistRail = railQuery(watchlist, (data) => data.items, showWatchlist);

  // The hero waits for what it picks from, so it never swaps a trending pick for an Up next one mid-view
  const settled = !upNext.isPending && (!showWatchlist || !watchlist.isPending) && !trending.isPending;
  const picks = useMemo(
    () =>
      heroPicks({
        upNext: upNext.data?.items ?? [],
        watchlist: showWatchlist ? (watchlist.data?.items ?? []) : [],
        trending: trending.data ?? [],
        date: localDateKey(today),
      }),
    [upNext.data, watchlist.data, trending.data, showWatchlist, today]
  );

  const rails = [upNextRail, newRail, watchlistRail];
  const allFailed = rails.every((rail, i) => rail.failed || (i === 2 && !showWatchlist));
  const nothing = rails.every((rail) => rail.items !== undefined && rail.items.length === 0);
  const retryAll = () => rails.forEach((rail) => rail.failed && rail.retry());

  const relinking = trakt.state === 'reauth' || trakt.state === 'pending';
  const watchlistCount = trakt.counts.watchlist || watchlistRail.items?.length || 0;

  return (
    <>
      <PageTitle />
      {!settled ? <HomeHeroSkeleton username={username} /> : picks.length > 0 && <HomeHero username={username} picks={picks} upNext={upNext.data?.items} />}
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
            Up next, new episodes and the watchlist didn’t load. Trending still works.
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
            {showWatchlist && (
              <TitleRail
                title="Your Trakt watchlist"
                subtitle={watchlistCount > 0 ? `${plural(watchlistCount, 'title')} · ${trakt.counts.watchlist_in_library} in your library` : undefined}
                query={watchlistRail}
                errorTitle="Couldn’t load your watchlist"
                errorDetail="momoshtrem didn’t answer. The rest of the page still works."
              />
            )}
          </>
        )}
        <TitleRail title="Trending this week" query={trendingRail} errorTitle="Couldn’t load trending titles" />
      </div>
    </>
  );
}

/**
 * For You (`/`, PLAN F5; #135): "What should I watch tonight, and is it
 * ready?" Which page it shows follows lib/for-you/state.ts: loading, first
 * time, Trakt off, the first sync, or the personal page. #9 adds its shelves
 * and Tonight's pick.
 */
export function ForYouView({ username }: { username: string }) {
  const integration = useTraktIntegration();
  const library = useQuery(libraryQuery({ page_size: 1 }));
  // A failed poll keeps what was known; only a first load that fails reads as "no Trakt"
  const trakt = integration.data ?? (integration.isError ? null : undefined);
  const state = forYouState({ trakt, libraryTotal: library.data?.summary.total ?? (library.isError ? null : undefined) });

  switch (state) {
    case 'loading':
      return <ForYouLoading username={username} />;
    case 'first_time':
      return <FirstTimeHome />;
    case 'trakt_off':
      return <TraktOffHome username={username} traktFailed={trakt === null} onRetryTrakt={() => void integration.refetch()} />;
    case 'first_sync':
      return trakt ? <FirstSyncHome trakt={trakt} /> : null;
    case 'default':
      return trakt ? <PersonalHome username={username} trakt={trakt} /> : null;
  }
}
