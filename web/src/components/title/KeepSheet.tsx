'use client';

import Link from 'next/link';
import { type FormEvent, useId, useState } from 'react';
import { useQuery, type UseQueryResult } from '@tanstack/react-query';
import { HardDrive, HardDriveDownload, Pin, PinOff, RefreshCw, Search } from 'lucide-react';
import { Banner, Button, LoadingRegion, Sheet, Skeleton, SkeletonText } from '@/components/ui';
import { useKeep } from '@/hooks/useKeep';
import { isApiError } from '@/lib/api/client';
import type { DownloadDuration, DownloadEstimate, StorageKeptSettings } from '@/lib/api/contract';
import { downloadEstimateQuery, isNotDownloadableError, isStorageFullError, seasonQuery, storageSettingsQuery, titleQuery } from '@/lib/api/endpoints';
import type { SheetComponentProps } from '@/lib/sheets/registry';
import { useSheet } from '@/lib/sheets/useSheet';
import { showSourceParams } from '@/lib/source/context';
import { STATUS } from '@/lib/status';
import {
  CONTRACT_KEEP_DEFAULTS,
  defaultKeepChoice,
  estimateCopy,
  type KeepChoice,
  keepCreateDuration,
  keepDuration,
  type KeepOption,
  keepOptions,
  NOT_DOWNLOADABLE_COPY,
  STORAGE_FULL_HINT,
  STORAGE_SETTINGS_HREF,
  storageFullLead,
} from '@/lib/title/keep';
import {
  heldByAction,
  heldByContext,
  heldByCopy,
  type KeepContext,
  keepContext,
  keepSeasonNumber,
  type KeepSheetMode,
  keepSheetMode,
  keepSheetParams,
  keepSubject,
  type KeepSubject,
} from '@/lib/title/keep-sheet';
import { KeepStatus } from './KeepStatus';

// The state pill says how far along it is: "Kept", not "Kept on disk" (it may still be downloading, or have no file)
const SHEET_TITLE: Record<KeepSheetMode, string> = { new: 'Download & keep', smart: 'Keep this smart pick', manage: 'Kept', held: 'Kept' };

/** 409 `storage_full`, from the estimate (before it's sent) or the POST: what it needs, what's left, and where to make room. */
function StorageFullBanner({ needed, available }: { needed: number; available: number }) {
  return (
    <Banner tone="warning" role="alert" icon={HardDrive} title={storageFullLead(needed, available)}>
      {STORAGE_FULL_HINT}{' '}
      <Link href={STORAGE_SETTINGS_HREF} className="text-accent-text underline underline-offset-2">
        Settings › Storage
      </Link>
      .
    </Banner>
  );
}

/** 422 `not_downloadable`: it plays from HDRezka; a torrent would make it keepable. */
function NotDownloadableBanner({ onFindTorrent }: { onFindTorrent: () => void }) {
  return (
    <Banner
      tone="warning"
      role="alert"
      title={STATUS.not_keepable.label}
      action={
        <Button size="sm" onClick={onFindTorrent}>
          <Search size={15} aria-hidden="true" /> Find a torrent
        </Button>
      }
    >
      {NOT_DOWNLOADABLE_COPY}
    </Banner>
  );
}

/** "Needs 14.2 GB · 61 GB left for kept downloads", or why it can't be kept. A 404 (no /downloads before #202) says nothing. */
function EstimateLine({ estimate, onFindTorrent }: { estimate: UseQueryResult<DownloadEstimate>; onFindTorrent: () => void }) {
  if (estimate.isPending) return <Skeleton shape="line" width={260} className="max-w-full" />;
  if (estimate.isError) {
    if (isApiError(estimate.error) && estimate.error.status === 404) return null;
    return <p className="t-small text-fg-3">Couldn’t check how much space it needs.</p>;
  }
  const copy = estimateCopy(estimate.data);
  if (copy.kind === 'full') return <StorageFullBanner needed={copy.needed} available={copy.available} />;
  if (copy.kind === 'not_downloadable') return <NotDownloadableBanner onFindTorrent={onFindTorrent} />;
  return (
    <p className="t-small flex items-start gap-2 text-fg-2">
      <HardDrive size={15} className="mt-0.5 shrink-0 text-fg-3" aria-hidden="true" />
      <span>{copy.text}</span>
    </p>
  );
}

/** The choices: radio cards with the default picked (nothing, when changing a Keep). */
function DurationChoices({ legend, value, onChange, options }: { legend: string; value: KeepChoice | null; onChange: (choice: KeepChoice) => void; options: KeepOption[] }) {
  const name = useId();
  return (
    <fieldset className="fs flex flex-col gap-2">
      <legend className="label mb-3">{legend}</legend>
      {options.map((option) => (
        <label key={option.value} className="radio-card">
          <input type="radio" name={name} value={option.value} checked={value === option.value} onChange={() => onChange(option.value)} />
          <span className="flex flex-col gap-0.5">
            <span className="font-semibold">{option.label}</span>
            {option.detail && <span className="t-small text-fg-2">{option.detail}</span>}
          </span>
        </label>
      ))}
    </fieldset>
  );
}

/** The source sheet on the same thing: a torrent makes an HDRezka title keepable. */
function sourceParamsFor(context: KeepContext): Record<string, string> {
  if (context.kind === 'movie') return { movie: String(context.tmdbId) };
  if (context.kind === 'show') return showSourceParams(context.tmdbId, {});
  if (context.kind === 'season') return showSourceParams(context.tmdbId, { season: context.season });
  return showSourceParams(context.tmdbId, { season: context.season, episode: context.episode });
}

/**
 * What POST sends for a choice. With the storage settings known, the default goes out empty, so
 * the server applies `kept.default_*` from its own clock; without them (they failed), the
 * choice is spelled out, as the contract's defaults the sheet shows.
 */
function createDuration(choice: KeepChoice, kept: Pick<StorageKeptSettings, 'default_until_watched' | 'default_days'> | null): DownloadDuration {
  return kept ? keepCreateDuration(choice, kept) : keepDuration(choice, CONTRACT_KEEP_DEFAULTS);
}

interface KeepBodyProps {
  context: KeepContext;
  subject: KeepSubject;
  mode: KeepSheetMode;
  formId: string;
  options: KeepOption[];
  estimate: UseQueryResult<DownloadEstimate>;
  choice: KeepChoice | null;
  onChoice: (choice: KeepChoice) => void;
  error: unknown;
  onFindTorrent: () => void;
  onSubmit: () => void;
}

function KeepBody({ context, subject, mode, formId, options, estimate, choice, onChoice, error, onFindTorrent, onSubmit }: KeepBodyProps) {
  const submit = (event: FormEvent) => {
    event.preventDefault();
    onSubmit();
  };

  // An episode its season or show keeps already: that keep is the one to change (the foot opens it)
  if (mode === 'held' && subject.heldBy) {
    const held = heldByCopy(subject.heldBy, keepSeasonNumber(context) ?? 0);
    return (
      <div className="flex flex-col gap-4">
        <KeepStatus keep={subject.heldBy} name={subject.name} />
        <p className="text-fg-2">{held ?? NOT_DOWNLOADABLE_COPY}</p>
      </div>
    );
  }

  return (
    <form id={formId} className="flex flex-col gap-5" onSubmit={submit} noValidate>
      {mode !== 'new' && subject.own && <KeepStatus keep={subject.own} name={subject.name} />}
      {mode === 'smart' && <p className="text-fg-2">Downloaded ahead because you’re watching the show. Keep it to choose how long it stays on disk.</p>}
      {mode !== 'manage' && <EstimateLine estimate={estimate} onFindTorrent={onFindTorrent} />}
      <DurationChoices legend={mode === 'manage' ? 'Change how long it’s kept' : 'Keep it on disk'} value={choice} onChange={onChoice} options={options} />
      {isStorageFullError(error) ? (
        <StorageFullBanner needed={error.details.needed_bytes} available={error.details.available_bytes} />
      ) : isNotDownloadableError(error) ? (
        <NotDownloadableBanner onFindTorrent={onFindTorrent} />
      ) : error ? (
        <Banner tone="error" role="alert" title={mode === 'manage' ? 'Couldn’t change it' : 'Couldn’t keep it'}>
          {error instanceof Error ? error.message : null}
        </Banner>
      ) : null}
    </form>
  );
}

/** Why there's nothing to keep: a link that names nothing, a title not in the library, or a server without downloads. */
const NOTHING_COPY = {
  bad_link: 'This link doesn’t point to anything that can be kept on disk.',
  missing: 'This isn’t in your library. Add it first, then it can be kept on disk.',
  unsupported: 'This momoshtrem can’t keep things on disk yet. Everything still streams to Infuse on demand.',
} as const;

/**
 * Download & keep (`?sheet=keep`, #206): keep a movie, a show, a season or
 * an episode on disk until it's watched (30 days at most, the default), for
 * 7, 30 or 90 days, or until removed. It says what the Keep needs against the
 * space kept downloads have left (GET /downloads/estimate) and why it can't
 * go ahead (409 `storage_full`, 422 `not_downloadable`). On something kept
 * already it changes the duration (PATCH) or stops keeping it (DELETE); on a
 * smart pick it keeps it or dismisses it; on an episode a season or show keep
 * holds, it opens that keep.
 */
export function KeepSheet({ params, close }: SheetComponentProps) {
  const formId = useId();
  const sheets = useSheet();
  const keep = useKeep();
  const context = keepContext(params);
  const mediaType = context?.kind === 'movie' ? 'movie' : 'tv';
  const tmdbId = context?.tmdbId ?? 0;
  const seasonNumber = context ? keepSeasonNumber(context) : null;

  const titleResult = useQuery({ ...titleQuery(mediaType, tmdbId), enabled: context !== null });
  const title = titleResult.data;
  const inLibrary = Boolean(title?.library.in_library);
  const seasonResult = useQuery({ ...seasonQuery(tmdbId, seasonNumber ?? 0), enabled: inLibrary && seasonNumber !== null });
  const storage = useQuery(storageSettingsQuery());

  const failed = titleResult.isError || seasonResult.isError;
  const loading = context !== null && !failed && (titleResult.isPending || (inLibrary && seasonNumber !== null && seasonResult.isPending));
  const result = context && title && !loading && !failed ? keepSubject(context, title, seasonResult.data) : null;
  const subject = result?.status === 'ready' ? result.subject : null;

  // While a Keep, a change or Stop keeping is sent (and the page refetches), the sheet stays as it was until it closes
  const [sending, setSending] = useState<KeepSheetMode | null>(null);
  const [picked, setPicked] = useState<KeepChoice | null>(null);
  const [error, setError] = useState<unknown>(null);
  // Opening the season's keep from a held episode reuses this sheet: start that one over
  const contextKey = context ? JSON.stringify(context) : '';
  const [shownKey, setShownKey] = useState(contextKey);
  if (shownKey !== contextKey) {
    setShownKey(contextKey);
    setSending(null);
    setPicked(null);
    setError(null);
  }

  const mode = sending ?? keepSheetMode(subject);
  // Until /settings/storage answers (or if it can't), the contract's defaults
  const defaults = storage.data?.kept ?? null;
  // A new Keep starts on the default; changing one starts on nothing, so Change duration waits for a pick
  const choice = picked ?? (mode === 'manage' ? null : defaultKeepChoice(defaults ?? CONTRACT_KEEP_DEFAULTS));
  // Asked once the target is known (the placeholder is never fetched), for a new Keep or a smart pick, and not while one is sent
  const estimate = useQuery({
    ...downloadEstimateQuery(subject?.target ?? { scope: 'movie', id: 0 }),
    enabled: subject !== null && (mode === 'new' || mode === 'smart') && sending === null,
  });
  // It would be 409 or 422: say so before it's sent
  const blocked = estimate.isSuccess && estimateCopy(estimate.data).kind !== 'fits';

  const findTorrent = () => {
    if (context) sheets.open('torrent', sourceParamsFor(context));
  };

  const submit = async () => {
    if (!subject || !choice) return;
    setError(null);
    setSending(mode);
    try {
      if (mode === 'manage' && subject.own) await keep.change(subject.own.download_id, keepDuration(choice, defaults ?? CONTRACT_KEEP_DEFAULTS), subject.name);
      else await keep.keep({ ...subject.target, ...createDuration(choice, defaults) }, subject.name);
      close();
    } catch (caught) {
      setSending(null);
      setError(caught);
    }
  };
  const stop = async () => {
    if (!subject?.own) return;
    setSending(mode);
    if (await keep.stop(subject.own, subject.name)) close();
    else setSending(null);
  };
  const openHeldBy = () => {
    if (subject?.heldBy && context && seasonNumber !== null) sheets.open('keep', keepSheetParams(heldByContext(context.tmdbId, subject.heldBy, seasonNumber)));
  };
  const retry = () => {
    if (titleResult.isError) void titleResult.refetch();
    if (seasonResult.isError) void seasonResult.refetch();
  };

  let footer;
  if (subject && mode === 'held' && subject.heldBy) {
    footer = (
      <Button variant="primary" className="grow" onClick={openHeldBy}>
        <Pin size={18} aria-hidden="true" /> {heldByAction(subject.heldBy, seasonNumber ?? 0)}
      </Button>
    );
  } else if (subject) {
    footer = (
      <>
        {mode !== 'new' && subject.own && (
          <Button variant="danger-quiet" className="grow" loading={keep.stopping} onClick={() => void stop()}>
            <PinOff size={16} aria-hidden="true" /> {subject.own.reason === 'smart' ? 'Dismiss smart pick' : 'Stop keeping'}
          </Button>
        )}
        <Button
          type="submit"
          form={formId}
          variant="primary"
          loading={keep.keeping || keep.changing}
          // A new Keep waits for the storage defaults, so the choice shown is the one the server applies
          disabled={!choice || (mode !== 'manage' && (blocked || storage.isPending))}
          className="grow"
        >
          {mode === 'manage' ? (
            'Change duration'
          ) : mode === 'smart' ? (
            <>
              <Pin size={18} aria-hidden="true" /> Keep it
            </>
          ) : (
            <>
              <HardDriveDownload size={18} aria-hidden="true" /> Download &amp; keep
            </>
          )}
        </Button>
      </>
    );
  }

  const nothing = !context ? NOTHING_COPY.bad_link : result?.status === 'unsupported' ? NOTHING_COPY.unsupported : NOTHING_COPY.missing;

  return (
    <Sheet open onOpenChange={(open) => !open && close()} title={SHEET_TITLE[mode]} description={subject?.name ?? title?.title} size="narrow" footer={footer}>
      {failed ? (
        <Banner
          tone="error"
          role="alert"
          title="Couldn’t load it"
          action={
            <Button size="sm" onClick={retry}>
              <RefreshCw size={15} aria-hidden="true" /> Try again
            </Button>
          }
        >
          {(titleResult.error ?? seasonResult.error)?.message}
        </Banner>
      ) : loading ? (
        <LoadingRegion label="Loading" className="flex flex-col gap-4">
          <Skeleton shape="line" width={240} />
          <SkeletonText lines={2} />
          {Array.from({ length: 5 }, (_, i) => (
            <Skeleton key={i} height={60} className="rounded-md" />
          ))}
        </LoadingRegion>
      ) : context && subject ? (
        <KeepBody
          context={context}
          subject={subject}
          mode={mode}
          formId={formId}
          options={keepOptions(defaults ?? CONTRACT_KEEP_DEFAULTS)}
          estimate={estimate}
          choice={choice}
          onChoice={(next) => {
            setPicked(next);
            setError(null);
          }}
          error={error}
          onFindTorrent={findTorrent}
          onSubmit={() => void submit()}
        />
      ) : (
        <p className="text-fg-2">{nothing}</p>
      )}
    </Sheet>
  );
}
