'use client';

import { type ClipboardEvent, type FormEvent, useCallback, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { History, MonitorPlay, X } from 'lucide-react';
import { SourceSheet, useSourceTitle, useSwitchSource } from '@/components/source';
import { Banner, Button, IconButton, LoadingRegion, SearchInput, Sheet, Skeleton, useConfirm } from '@/components/ui';
import { useStreamMovie } from '@/hooks/useHDRezka';
import { isApiError } from '@/lib/api/client';
import type { HDRezkaCandidate, Title } from '@/lib/api/contract';
import { hdrezkaSearchQuery, hdrezkaStreamsQuery, hdrezkaTitleQuery } from '@/lib/api/endpoints';
import { hdrezkaLink, hdrezkaProblem, replaceFileConfirm } from '@/lib/hdrezka/copy';
import { defaultQuality, defaultTranslator, qualityNote, translatorKey } from '@/lib/hdrezka/pick';
import { clearDraft, type HDRezkaDraft, type HDRezkaPick, loadDraft, saveDraft } from '@/lib/hdrezka/session';
import type { SheetComponentProps } from '@/lib/sheets/registry';
import { contextMediaType, type SourceContext, sourceContext } from '@/lib/source/context';
import { MatchStep } from './MatchStep';
import { PickedMatch, QualityStep, VoiceOverStep } from './StreamSteps';

/** HDRezka keeps the original title next to the Russian one, so search by it first. */
const searchName = (title: Title) => title.original_title ?? title.title;

/** Where a new visit starts: the movie's current stream when it has one, else the search. */
function freshDraft(title: Title): HDRezkaDraft {
  const stream = title.media_type === 'movie' ? title.library.stream : null;
  return {
    input: searchName(title),
    query: null,
    pick: stream ? { url: stream.url, title: stream.title } : null,
    translator: null,
    quality: null,
  };
}

const toPick = ({ url, title, original_title, year, category }: HDRezkaCandidate): HDRezkaPick => ({ url, title, original_title, year, category });

/** The path of a title page on any mirror, to compare picks with what plays now. */
const pagePath = (url: string) => url.replace(/^https?:\/\/[^/]+/i, '').replace(/[?#].*$/, '');

interface PickerProps {
  context: SourceContext;
  title: Title;
  close: () => void;
}

/**
 * Match → voice-over → quality, then "Use HDRezka" in the sticky foot.
 * Everything picked lives in a draft that's kept per context for the
 * session (`lib/hdrezka/session.ts`). The show steps (season, episodes,
 * chunked assign) slot in after the quality.
 */
function HDRezkaPicker({ context, title, close }: PickerProps) {
  const [restored] = useState(() => loadDraft(context.key) !== null);
  const [draft, setDraft] = useState<HDRezkaDraft>(() => loadDraft(context.key) ?? freshDraft(title));
  const switchTo = useSwitchSource(context, close, title);
  const confirm = useConfirm();
  const streamMovie = useStreamMovie(title.title);
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
    streamMovie.reset();
    update({ ...patch, pick, translator: null, quality: null });
  };

  const { pick } = draft;
  const libraryId = title.library.library_id ?? undefined;
  const search = useQuery({
    ...hdrezkaSearchQuery({ media_type: contextMediaType(context), tmdb_id: title.tmdb_id, q: draft.query ?? undefined }),
    enabled: pick === null,
  });
  const hdTitle = useQuery({
    ...hdrezkaTitleQuery({
      url: pick?.url ?? '',
      ...(context.kind === 'movie' ? { movie_id: libraryId } : { show_id: libraryId }),
    }),
    enabled: pick !== null,
  });
  const translator = hdTitle.data ? defaultTranslator(hdTitle.data, draft.translator) : null;
  const streams = useQuery({
    ...hdrezkaStreamsQuery({ url: pick?.url ?? '', translator_id: translator?.id ?? 0 }),
    enabled: pick !== null && translator !== null,
  });
  const quality = streams.data ? defaultQuality(streams.data.qualities, draft.quality) : null;

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    const link = hdrezkaLink(draft.input);
    if (link) choose({ url: link });
    else update({ query: draft.input.trim() || null, pick: null, translator: null, quality: null });
  };
  // A pasted link goes straight to that title
  const onPaste = (event: ClipboardEvent<HTMLInputElement>) => {
    const text = event.clipboardData.getData('text');
    const link = hdrezkaLink(text);
    if (!link) return;
    event.preventDefault();
    choose({ url: link }, { input: text.trim() });
  };
  const searchFor = (query: string) => update({ input: query, query, pick: null, translator: null, quality: null });
  const focusInput = () => {
    inputRef.current?.focus();
    inputRef.current?.select();
  };

  const movie = title.media_type === 'movie' ? title : null;
  const current = movie?.library.stream ?? null;
  const playingNow =
    current !== null && pick !== null && pagePath(current.url) === pagePath(pick.url) && current.translator === translator?.name && current.quality === quality?.label;
  const ready = movie !== null && libraryId !== undefined && pick !== null && translator !== null && quality !== null && !playingNow;

  const assign = async () => {
    if (!ready || !movie || !pick || !translator || !quality || libraryId === undefined) return;
    if (movie.library.torrent || current) {
      const words = replaceFileConfirm(movie.title, { torrent: movie.library.torrent !== null, stream: current }, { translator: translator.name, quality: quality.label });
      if (!(await confirm({ ...words, tone: 'primary' }))) return;
    }
    streamMovie.mutate(
      { movieId: libraryId, body: { url: pick.url, translator_id: translator.id, quality: quality.label } },
      {
        onSuccess: () => {
          clearDraft(context.key);
          close();
        },
      }
    );
  };

  const summary = [translator?.name, quality?.label, quality ? qualityNote(quality) : null].filter(Boolean).join(' · ');
  const footer =
    movie && pick ? (
      <>
        <p className="hd-foot-summary t-small text-fg-2" lang="ru">
          {playingNow ? 'This is what Infuse plays now' : libraryId === undefined ? 'Add the movie to your library first' : summary}
        </p>
        <Button variant="primary" size="lg" className="shrink-0" disabled={!ready} loading={streamMovie.isPending} onClick={() => void assign()}>
          <MonitorPlay size={20} aria-hidden="true" /> Use HDRezka
        </Button>
      </>
    ) : undefined;

  return (
    <SourceSheet
      tab="hdrezka"
      context={context}
      close={close}
      footer={footer}
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
          query={draft.query ?? search.data?.query ?? searchName(title)}
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
              streamMovie.reset();
              update({ pick: null, translator: null, quality: null });
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
            />
          )}
          {context.kind === 'show' && (
            <Banner title="Episodes from HDRezka aren’t here yet">Use the Torrents tab for this show for now.</Banner>
          )}
          {streamMovie.isError && <AssignError error={streamMovie.error} />}
        </>
      )}
    </SourceSheet>
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
 * voice-over and a quality, and Infuse plays that stream.
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
