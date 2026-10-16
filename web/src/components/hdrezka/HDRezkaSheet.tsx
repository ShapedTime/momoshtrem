'use client';

import { type ClipboardEvent, type FormEvent, type ReactNode, useCallback, useRef, useState } from 'react';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { History, MonitorPlay, X } from 'lucide-react';
import { SourceSheet, useSourceTitle, useSwitchSource } from '@/components/source';
import { Banner, Button, IconButton, LoadingRegion, Progress, SearchInput, Sheet, Skeleton, useConfirm } from '@/components/ui';
import { useStreamEpisodes, useStreamMovie } from '@/hooks/useHDRezka';
import { isApiError } from '@/lib/api/client';
import type { HDRezkaCandidate, HDRezkaQuality, HDRezkaSeason, HDRezkaTitle, HDRezkaTranslator, MovieTitle, ShowTitle, Title } from '@/lib/api/contract';
import { hdrezkaSearchQuery, hdrezkaStreamsQuery, hdrezkaTitleQuery } from '@/lib/api/endpoints';
import { hdrezkaLink, hdrezkaProblem, replaceEpisodesConfirm, replaceFileConfirm, searchNameFor } from '@/lib/hdrezka/copy';
import {
  defaultSeasonNumber,
  defaultSelection,
  type EpisodeScope,
  episodeKey,
  episodeTargets,
  libraryCode,
  sampleEpisode,
} from '@/lib/hdrezka/episodes';
import { defaultQuality, defaultTranslator, qualityNote, translatorCut, translatorKey } from '@/lib/hdrezka/pick';
import { rememberedVoiceOver } from '@/lib/hdrezka/remembered';
import { clearDraft, type HDRezkaDraft, type HDRezkaPick, loadDraft, saveDraft } from '@/lib/hdrezka/session';
import type { SheetComponentProps } from '@/lib/sheets/registry';
import { contextMediaType, type SourceContext, sourceContext } from '@/lib/source/context';
import { plural } from '@/lib/title/format';
import { EpisodesStep, FailedEpisodes } from './EpisodesStep';
import { MatchStep } from './MatchStep';
import { PickedMatch, QualityStep, VoiceOverStep } from './StreamSteps';


/** Where a new visit starts: the movie's current stream when it has one, else the search. */
function freshDraft(title: Title): HDRezkaDraft {
  const stream = title.media_type === 'movie' ? title.library.stream : null;
  return {
    input: searchNameFor(title),
    query: null,
    pick: stream ? { url: stream.url, title: stream.title } : null,
    translator: null,
    quality: null,
  };
}

/** Changing the match starts the rest over. */
const NOTHING_PICKED = { translator: null, quality: null, season: null, episodes: null } satisfies Partial<HDRezkaDraft>;

const toPick = ({ url, title, original_title, year, category }: HDRezkaCandidate): HDRezkaPick => ({ url, title, original_title, year, category });

/** The path of a title page on any mirror, to compare picks with what plays now. */
const pagePath = (url: string) => url.replace(/^https?:\/\/[^/]+/i, '').replace(/[?#].*$/, '');

/**
 * The voice-over this title streams now: a movie's stream when it's from
 * this page; for a show, the scope's episode's, else any of its episodes'.
 */
function currentVoiceOver(title: Title, hdTitle: HDRezkaTitle | undefined, scope: EpisodeScope): string | null {
  if (!hdTitle) return null;
  if (title.media_type === 'movie') {
    const stream = title.library.stream;
    return stream && pagePath(stream.url) === hdTitle.path ? stream.translator : null;
  }
  const streamed = hdTitle.seasons.flatMap((s) => s.episodes.flatMap((e) => (e.library?.stream ? [e.library] : [])));
  const inScope = streamed.find((library) => library.season === scope.season && library.episode === scope.episode);
  return (inScope ?? streamed[0])?.stream?.translator ?? null;
}

/** What a flow adds to the picker: steps after the quality, and the sticky foot. */
interface FlowParts {
  steps: ReactNode;
  foot: ReactNode;
}

interface Picked {
  pick: HDRezkaPick | null;
  translator: HDRezkaTranslator | null;
  quality: HDRezkaQuality | null;
}

/** The foot: what's picked (or how far a run is), then the primary action. */
function Foot({ summary, children }: { summary: ReactNode; children: ReactNode }) {
  return (
    <>
      <div className="hd-foot-summary t-small text-fg-2" lang="ru">
        {summary}
      </div>
      {children}
    </>
  );
}

/** Assigning failed: premium explains itself, anything else is an error to retry with the same button. */
function AssignError({ error }: { error: unknown }) {
  const problem = hdrezkaProblem(error);
  const premium = isApiError(error) && error.code === 'premium_only';
  return (
    <Banner tone={premium ? 'warning' : 'error'} role="alert" title={problem.title}>
      {problem.body}
    </Banner>
  );
}

/** A movie: "Use HDRezka", asking first when it has a file, and saying when the pick is what plays now. */
function useMovieFlow(movie: MovieTitle | null, context: SourceContext, picked: Picked, close: () => void): FlowParts | null {
  const confirm = useConfirm();
  const streamMovie = useStreamMovie(movie?.title ?? '');
  if (!movie) return null;
  const { pick, translator, quality } = picked;
  const libraryId = movie.library.library_id;
  const current = movie.library.stream;
  const playingNow =
    current !== null && pick !== null && pagePath(current.url) === pagePath(pick.url) && current.translator === translator?.name && current.quality === quality?.label;
  const ready = libraryId !== null && pick !== null && translator !== null && quality !== null && !playingNow;

  const assign = async () => {
    if (!ready || !pick || !translator || !quality || libraryId === null) return;
    if (movie.library.torrent || current) {
      const words = replaceFileConfirm(movie.title, { torrent: movie.library.torrent !== null, stream: current }, { translator: translator.name, quality: quality.label });
      if (!(await confirm({ ...words, tone: 'primary' }))) return;
    }
    streamMovie.mutate(
      { movieId: libraryId, body: { url: pick.url, translator_id: translator.id, ...translatorCut(translator), quality: quality.label } },
      {
        onSuccess: () => {
          clearDraft(context.key);
          close();
        },
      }
    );
  };

  const summary = playingNow
    ? 'This is what Infuse plays now'
    : libraryId === null
      ? 'Add the movie to your library first'
      : [translator?.name, quality?.label, quality ? qualityNote(quality) : null].filter(Boolean).join(' · ');
  return {
    steps: streamMovie.isError ? <AssignError error={streamMovie.error} /> : null,
    foot: pick && (
      <Foot summary={summary}>
        <Button variant="primary" size="lg" className="shrink-0" disabled={!ready} loading={streamMovie.isPending} onClick={() => void assign()}>
          <MonitorPlay size={20} aria-hidden="true" /> Use HDRezka
        </Button>
      </Foot>
    ),
  };
}

interface ShowFlowInput extends Picked {
  seasons: HDRezkaSeason[];
  seasonsStatus: 'pending' | 'error' | 'success';
  seasonsError: unknown;
  retrySeasons: () => void;
  seasonNumber: number | null;
  selected: string[];
  /** The draft's update. */
  update: (patch: Partial<HDRezkaDraft>) => void;
}

/**
 * A show: the episodes step, then "Stream N episodes", assigned 30 at a
 * time with progress. Episodes that failed stay selected, with Retry for
 * only them; once all made it, the sheet closes.
 */
function useShowFlow(show: ShowTitle | null, context: SourceContext, input: ShowFlowInput, close: () => void): FlowParts | null {
  const confirm = useConfirm();
  const episodes = useStreamEpisodes(show?.title ?? '');
  if (!show) return null;
  const { pick, translator, quality, seasons, selected, update } = input;
  const showId = show.library.library_id;
  const targets = episodeTargets(seasons, selected);
  const ready = showId !== null && pick !== null && translator !== null && quality !== null && targets.length > 0 && !episodes.running;
  const codes = new Map(seasons.flatMap((s) => s.episodes.flatMap((e) => (e.library ? [[e.library.episode_id, libraryCode(e.library)] as const] : []))));

  const stream = async (retry: boolean) => {
    if (!ready || !pick || !translator || !quality || showId === null) return;
    const failedIds = new Set(episodes.failed.map((f) => f.episode_id));
    const run = retry ? targets.filter((t) => failedIds.has(t.episode_id)) : targets;
    const withFile = run.filter((t) => t.library.has_file).length;
    // A retry was already confirmed
    if (!retry && withFile > 0) {
      const words = replaceEpisodesConfirm(show.title, { count: run.length, withFile, code: run.length === 1 ? libraryCode(run[0].library) : null });
      if (!(await confirm({ ...words, tone: 'primary' }))) return;
    }
    const result = await episodes.run(
      showId,
      { url: pick.url, translator_id: translator.id, quality: quality.label },
      run.map(({ episode_id, season, episode }) => ({ episode_id, season, episode })),
      translator
    );
    if (result.failed.length === 0) {
      clearDraft(context.key);
      close();
      return;
    }
    // Only what failed stays selected, ready for Retry
    const failed = new Set(result.failed.map((f) => f.episode_id));
    update({ episodes: run.filter((t) => failed.has(t.episode_id)).map((t) => episodeKey(t.season, t.episode)) });
  };

  const { progress } = episodes;
  const summary = progress ? (
    <div className="flex flex-col gap-1.5">
      <span className="tabular-nums">
        Streaming {progress.done} of {plural(progress.total, 'episode')}…
      </span>
      <Progress value={progress.total ? Math.round((progress.done / progress.total) * 100) : 0} label="Streaming episodes from HDRezka" />
    </div>
  ) : showId === null ? (
    'Add the show to your library first'
  ) : (
    [translator?.name, quality?.label].filter(Boolean).join(' · ')
  );

  return {
    steps: (
      <>
        {translator && (
          <EpisodesStep
            status={input.seasonsStatus}
            seasons={seasons}
            error={input.seasonsError}
            onRetry={input.retrySeasons}
            seasonNumber={input.seasonNumber}
            onSeasonChange={(season) => update({ season })}
            selected={selected}
            onSelectedChange={(keys) => update({ episodes: keys })}
          />
        )}
        {episodes.failed.length > 0 && !episodes.running && (
          <FailedEpisodes failed={episodes.failed} codes={codes} partial={episodes.lastAssigned > 0} retrying={false} onRetry={() => void stream(true)} />
        )}
      </>
    ),
    foot: pick && (
      <Foot summary={summary}>
        <Button variant="primary" size="lg" className="shrink-0" disabled={!ready} loading={episodes.running} onClick={() => void stream(false)}>
          <MonitorPlay size={20} aria-hidden="true" /> {targets.length === 0 ? 'Stream episodes' : `Stream ${plural(targets.length, 'episode')}`}
        </Button>
      </Foot>
    ),
  };
}

interface PickerProps {
  context: SourceContext;
  title: Title;
  close: () => void;
}

/**
 * Match → voice-over → quality (→ episodes, for a show), then the primary
 * action in the sticky foot. Everything picked lives in a draft that's kept
 * per context for the session (`lib/hdrezka/session.ts`).
 */
function HDRezkaPicker({ context, title, close }: PickerProps) {
  const [restored] = useState(() => loadDraft(context.key) !== null);
  const [draft, setDraft] = useState<HDRezkaDraft>(() => loadDraft(context.key) ?? freshDraft(title));
  const [remembered] = useState(rememberedVoiceOver);
  const switchTo = useSwitchSource();
  const inputRef = useRef<HTMLInputElement>(null);
  const focusVoiceOver = useRef(false);
  const voiceOverRef = useCallback((heading: HTMLHeadingElement | null) => {
    if (heading && focusVoiceOver.current) {
      focusVoiceOver.current = false;
      heading.focus();
    }
  }, []);

  /** Every change is kept for the session (only changes: just opening and closing isn't "where you left off"). */
  const update = (patch: Partial<HDRezkaDraft>) => {
    const next = { ...draft, ...patch };
    setDraft(next);
    saveDraft(context.key, next);
  };
  const choose = (pick: HDRezkaPick, patch: Partial<HDRezkaDraft> = {}) => {
    focusVoiceOver.current = true;
    update({ ...patch, ...NOTHING_PICKED, pick });
  };

  const { pick } = draft;
  const movie = title.media_type === 'movie' ? title : null;
  const show = title.media_type === 'tv' ? title : null;
  const scope: EpisodeScope = context.kind === 'show' ? { season: context.season, episode: context.episode } : { season: null, episode: null };
  const libraryId = title.library.library_id ?? undefined;
  const search = useQuery({
    ...hdrezkaSearchQuery({ media_type: contextMediaType(context), tmdb_id: title.tmdb_id, q: draft.query ?? undefined }),
    enabled: pick === null,
  });
  const hdTitle = useQuery({
    ...hdrezkaTitleQuery({ url: pick?.url ?? '', ...(show ? { show_id: libraryId } : { movie_id: libraryId }) }),
    enabled: pick !== null,
  });
  const current = currentVoiceOver(title, hdTitle.data, scope);
  const translator = hdTitle.data ? defaultTranslator(hdTitle.data, { keep: draft.translator, current, remembered }) : null;

  // A series lists its episodes per voice-over, and the first answer is for the page's default
  const otherVoiceOver = show !== null && translator !== null && hdTitle.data !== undefined && translator.id !== hdTitle.data.translator_id;
  const episodesTitle = useQuery({
    ...hdrezkaTitleQuery({ url: pick?.url ?? '', translator_id: translator?.id, show_id: libraryId }),
    enabled: pick !== null && otherVoiceOver,
  });
  const episodesSource = otherVoiceOver ? episodesTitle : hdTitle;
  const seasons = episodesSource.data?.seasons ?? [];
  // The default selection follows the first tab, not the open one: switching tabs never changes it
  const firstSeason = defaultSeasonNumber(seasons, scope, show?.default_season ?? null);
  const seasonNumber = draft.season ?? firstSeason;
  const selected = draft.episodes ?? defaultSelection(seasons, firstSeason, scope);
  // A show's qualities stand for one episode: the first picked, else the open season's first
  const sample = show ? sampleEpisode(seasons, seasonNumber, selected) : null;

  const streams = useQuery({
    ...hdrezkaStreamsQuery({
      url: pick?.url ?? '',
      translator_id: translator?.id ?? 0,
      ...(translator ? translatorCut(translator) : {}),
      ...(sample ?? {}),
    }),
    enabled: pick !== null && translator !== null && (!show || sample !== null),
    // Picking episodes can move the sample: keep the qualities on screen meanwhile
    placeholderData: show ? keepPreviousData : undefined,
  });
  // Reopened on what a movie plays now: start on its quality too
  const stream = movie?.library.stream ?? null;
  const keepQuality = draft.quality ?? (stream && translator && current === translator.name ? stream.quality : null);
  const quality = streams.data ? defaultQuality(streams.data.qualities, keepQuality) : null;

  const picked: Picked = { pick, translator, quality };
  const movieFlow = useMovieFlow(movie, context, picked, close);
  const showFlow = useShowFlow(
    show,
    context,
    {
      ...picked,
      seasons,
      seasonsStatus: episodesSource.status,
      seasonsError: episodesSource.error,
      retrySeasons: () => void episodesSource.refetch(),
      seasonNumber,
      selected,
      update,
    },
    close
  );
  const flow = movieFlow ?? showFlow;

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    const link = hdrezkaLink(draft.input);
    if (link) choose({ url: link });
    else update({ ...NOTHING_PICKED, query: draft.input.trim() || null, pick: null });
  };
  // A pasted link goes straight to that title
  const onPaste = (event: ClipboardEvent<HTMLInputElement>) => {
    const text = event.clipboardData.getData('text');
    const link = hdrezkaLink(text);
    if (!link) return;
    event.preventDefault();
    choose({ url: link }, { input: text.trim() });
  };
  const searchFor = (query: string) => update({ ...NOTHING_PICKED, input: query, query, pick: null });
  const focusInput = () => {
    inputRef.current?.focus();
    inputRef.current?.select();
  };

  return (
    <SourceSheet
      tab="hdrezka"
      context={context}
      close={close}
      footer={flow?.foot || undefined}
      head={
        <form role="search" onSubmit={onSubmit} aria-label="Search HDRezka">
          <SearchInput
            ref={inputRef}
            aria-label="Search HDRezka, or paste a link"
            placeholder="Title, or paste an HDRezka link"
            autoComplete="off"
            enterKeyHint="search"
            value={draft.input}
            onChange={(event) => update({ input: event.target.value })}
            onPaste={onPaste}
            end={
              draft.input ? (
                <IconButton
                  size="sm"
                  aria-label="Clear search"
                  onClick={() => {
                    update({ input: '' });
                    inputRef.current?.focus();
                  }}
                >
                  <X size={16} aria-hidden="true" />
                </IconButton>
              ) : undefined
            }
          />
        </form>
      }
    >
      {restored && (
        <p className="t-small flex items-center gap-1.5 text-fg-3">
          <History size={14} aria-hidden="true" /> Picked up where you left off
        </p>
      )}
      {pick === null ? (
        <MatchStep
          target={title}
          query={draft.query ?? search.data?.query ?? searchNameFor(title)}
          status={search.status}
          items={search.data?.items ?? []}
          error={search.error}
          onRetry={() => void search.refetch()}
          onUse={(candidate) => choose(toPick(candidate))}
          onSearch={searchFor}
          onPasteLink={focusInput}
          onTryTorrents={() => switchTo('torrent')}
        />
      ) : (
        <>
          <PickedMatch
            pick={pick}
            title={hdTitle.data}
            onChange={() => {
              update({ ...NOTHING_PICKED, pick: null });
              focusInput();
            }}
          />
          <VoiceOverStep
            status={hdTitle.status}
            title={hdTitle.data}
            error={hdTitle.error}
            onRetry={() => void hdTitle.refetch()}
            selected={translator}
            onSelect={(t) => update({ translator: translatorKey(t) })}
            onTryTorrents={() => switchTo('torrent')}
            headingRef={voiceOverRef}
          />
          {translator && (
            <QualityStep
              status={streams.status}
              streams={streams.data}
              error={streams.error}
              onRetry={() => void streams.refetch()}
              selected={quality}
              onSelect={(q) => update({ quality: q.label })}
              perEpisode={show !== null}
            />
          )}
          {flow?.steps}
        </>
      )}
    </SourceSheet>
  );
}

/** Until the title is here (a deep link): the head's place and skeletons, at full height. */
function PickerSkeleton({ context, close }: { context: SourceContext; close: () => void }) {
  return (
    <SourceSheet tab="hdrezka" context={context} close={close} head={<Skeleton height={44} />}>
      <LoadingRegion label="Loading" className="flex flex-col gap-3">
        <Skeleton height={140} />
        <Skeleton height={110} />
      </LoadingRegion>
    </SourceSheet>
  );
}

function HDRezkaContext({ context, close }: { context: SourceContext; close: () => void }) {
  const { title } = useSourceTitle(context);
  if (!title) return <PickerSkeleton context={context} close={close} />;
  return <HDRezkaPicker context={context} title={title} close={close} />;
}

/**
 * The HDRezka tab of the source sheet (`?sheet=hdrezka&movie=<tmdb>`, or
 * `&show=<tmdb>&season=n&se=S01E29`; #67): find the title on HDRezka, pick a
 * voice-over and a quality (and a show's episodes), and Infuse plays that
 * stream.
 */
export function HDRezkaSheet({ params, close }: SheetComponentProps) {
  const context = sourceContext(params);
  if (!context) {
    return (
      <Sheet open onOpenChange={(open) => !open && close()} title="Find a source" size="narrow">
        <p className="text-fg-2">This link doesn’t say which movie or show to find a source for. Open it from a title’s page.</p>
      </Sheet>
    );
  }
  return <HDRezkaContext key={context.key} context={context} close={close} />;
}
