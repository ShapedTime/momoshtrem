'use client';

import { Download, RefreshCw, X } from 'lucide-react';
import { Banner, Button, Chip, IconButton, LoadingRegion, ReleaseName, Select, Skeleton, StatusPill } from '@/components/ui';
import type { Subtitle, SubtitleCandidate } from '@/lib/api/contract';
import { STATUS } from '@/lib/status';
import { languageName, plural } from '@/lib/title/format';
import { isStored, matchStatus, SUBTITLE_LANGUAGES } from '@/lib/title/subtitles';

export type SubtitleSearchState =
  | { status: 'idle' }
  | { status: 'pending'; language: string }
  | { status: 'error'; language: string; message: string; retry: () => void }
  | { status: 'success'; language: string; items: SubtitleCandidate[] };

export interface SubtitlesPanelProps {
  /** Stored subtitles; undefined while they load. */
  stored: Subtitle[] | undefined;
  storedError: boolean;
  onRetryStored: () => void;
  /** Downloaded subtitles only: torrent ones stay with the file. */
  onRemove: (subtitle: Subtitle) => void;
  removingId: number | null;
  /** The language the search will ask for. */
  language: string;
  onLanguageChange: (code: string) => void;
  onSearch: () => void;
  search: SubtitleSearchState;
  onGet: (candidate: SubtitleCandidate) => void;
  gettingFileId: number | null;
}

function storedDetail(subtitle: Subtitle): string {
  if (subtitle.source === 'torrent') return `From the torrent · ${subtitle.format.toUpperCase()}`;
  return `OpenSubtitles · ${STATUS[matchStatus(subtitle.match)].label}`;
}

function StoredList({ stored, onRemove, removingId }: Pick<SubtitlesPanelProps, 'onRemove' | 'removingId'> & { stored: Subtitle[] }) {
  if (!stored.length) {
    return <p className="t-small px-4 pb-4 text-fg-3">No subtitles yet. Get some below and Infuse offers them with the video.</p>;
  }
  return (
    <ul aria-label="Your subtitles">
      {stored.map((subtitle) => (
        <li key={subtitle.id} className="list-row">
          <span className="lang" aria-hidden="true">
            {subtitle.language_code}
          </span>
          <div className="grow">
            <span className="font-medium text-fg-1">{subtitle.language_name}</span>
            <span className="t-small text-fg-3">{storedDetail(subtitle)}</span>
          </div>
          {subtitle.source === 'opensubtitles' && (
            <IconButton
              size="sm"
              aria-label={`Remove ${subtitle.language_name} subtitles`}
              loading={removingId === subtitle.id}
              onClick={() => onRemove(subtitle)}
            >
              <X size={16} aria-hidden="true" />
            </IconButton>
          )}
        </li>
      ))}
    </ul>
  );
}

function Results({ search, stored, onGet, gettingFileId }: Pick<SubtitlesPanelProps, 'search' | 'onGet' | 'gettingFileId'> & { stored: Subtitle[] }) {
  switch (search.status) {
    case 'idle':
      return null;
    case 'pending':
      return (
        <LoadingRegion label={`Searching ${languageName(search.language)} subtitles`} className="flex flex-col gap-2">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} height={74} className="rounded-md" />
          ))}
        </LoadingRegion>
      );
    case 'error':
      return (
        <Banner
          tone="error"
          role="alert"
          title="Couldn’t search OpenSubtitles"
          action={
            <Button size="sm" onClick={search.retry}>
              <RefreshCw size={15} aria-hidden="true" /> Try again
            </Button>
          }
        >
          {search.message}
        </Banner>
      );
    case 'success': {
      const language = languageName(search.language);
      if (!search.items.length) return <p className="t-small text-fg-2">OpenSubtitles has no {language} subtitles for this yet.</p>;
      return (
        <ul className="flex flex-col gap-2" aria-label={`${language} subtitles on OpenSubtitles`}>
          {search.items.map((candidate) => (
            <li key={candidate.file_id} className="sub-result">
              <div className="grow">
                <ReleaseName name={candidate.file_name} className="text-fg-2" />
                <span className="flex flex-wrap items-center gap-2">
                  <StatusPill status={matchStatus(candidate.match)} />
                  <span className="t-small text-fg-3">{plural(candidate.downloads, 'download')}</span>
                  {candidate.hearing_impaired && <Chip>Hearing impaired</Chip>}
                  {candidate.machine_translated && <Chip tone="warn">Machine translated</Chip>}
                </span>
                <span className="t-small text-fg-3">{candidate.match_reason}</span>
              </div>
              {isStored(candidate, stored) ? (
                <StatusPill status="added" />
              ) : (
                <Button size="sm" loading={gettingFileId === candidate.file_id} onClick={() => onGet(candidate)}>
                  <Download size={15} aria-hidden="true" /> Get
                  <span className="sr-only"> {candidate.file_name}</span>
                </Button>
              )}
            </li>
          ))}
        </ul>
      );
    }
  }
}

/**
 * Subtitles for one movie or episode (B7): what Infuse already has (torrent
 * subtitles can't be removed), and an OpenSubtitles search ranked by how
 * well each file matches the release you have.
 */
export function SubtitlesPanel(props: SubtitlesPanelProps) {
  const { stored, storedError, onRetryStored, language, onLanguageChange, onSearch, search } = props;
  const languages = stored ? new Set(stored.map((subtitle) => subtitle.language_code)).size : 0;
  return (
    <section className="panel subs flex flex-col" aria-labelledby="subs-h">
      <div className="panel-head">
        <h2 className="t-title-3" id="subs-h">
          Subtitles
        </h2>
        {stored && <span className="t-small text-fg-3">{plural(languages, 'language')}</span>}
      </div>
      {stored ? (
        <StoredList stored={stored} onRemove={props.onRemove} removingId={props.removingId} />
      ) : storedError ? (
        <div className="px-4 pb-4">
          <Banner
            tone="error"
            title="Couldn’t load the subtitles"
            action={
              <Button size="sm" onClick={onRetryStored}>
                Try again
              </Button>
            }
          />
        </div>
      ) : (
        <LoadingRegion label="Loading subtitles" className="flex flex-col gap-2 px-4 pb-4">
          <Skeleton height={44} />
          <Skeleton height={44} />
        </LoadingRegion>
      )}
      <div className="search panel-pad flex flex-col gap-3">
        <form
          className="flex gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            onSearch();
          }}
        >
          <Select aria-label="Subtitle language" value={language} onChange={(event) => onLanguageChange(event.target.value)} className="min-w-0 flex-1">
            {SUBTITLE_LANGUAGES.map((code) => (
              <option key={code} value={code}>
                {languageName(code)}
              </option>
            ))}
          </Select>
          <Button type="submit" loading={search.status === 'pending'}>
            Search
          </Button>
        </form>
        <p className="t-small text-fg-3">OpenSubtitles · ranked by how well they match your release</p>
        <Results search={search} stored={stored ?? []} onGet={props.onGet} gettingFileId={props.gettingFileId} />
      </div>
    </section>
  );
}
