import type { ReactNode } from 'react';
import { Check, CircleAlert, Layers, LoaderCircle, MonitorPlay, Search, TriangleAlert } from 'lucide-react';
import { CatEyes } from '@/components/brand';
import { Banner, Button, EmptyState, Skeleton } from '@/components/ui';
import type { IndexerReport, SearchMissingRun, SearchQuery, SearchScope } from '@/lib/api/contract';
import {
  indexerProgress,
  missingCopy,
  type NoResultsSuggestions,
  noResultsBody,
  noResultsTitle,
  partialProblem,
  type SearchProblem,
  searchingDetail,
  searchingTitle,
} from '@/lib/search/copy';
import { cx } from '@/lib/utils/cx';

/** The torrent search's states around the results (HANDOFF §5.2). */

const INDEXER_ICON = {
  ok: <Check size={15} aria-hidden="true" className="text-ready" />,
  pending: <LoaderCircle size={15} aria-hidden="true" className="animate-spin text-accent-text motion-reduce:animate-none" />,
  timeout: <TriangleAlert size={15} aria-hidden="true" className="text-warning" />,
  error: <CircleAlert size={15} aria-hidden="true" className="text-error" />,
} satisfies Record<IndexerReport['status'], ReactNode>;

/** Each indexer: a check and "26 results", a spinner and "waiting · 6 s", or why it failed. */
export function IndexerList({ indexers }: { indexers: IndexerReport[] }) {
  return (
    <ul className="ts-indexers" aria-label="Indexers">
      {indexers.map((indexer) => (
        <li key={indexer.name} className={cx(indexer.status !== 'ok' && 'text-fg-2')}>
          {INDEXER_ICON[indexer.status]}
          <span className="min-w-0 truncate text-fg-1">{indexer.name}</span>
          <span className="t-num text-fg-3">{indexerProgress(indexer)}</span>
        </li>
      ))}
    </ul>
  );
}

/** Three cards the size of real ones (name, chips, facts, the actions' column). */
export function ReleaseSkeletons({ count = 3 }: { count?: number }) {
  return (
    <div className="flex flex-col gap-3" aria-hidden="true">
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="release ts-release">
          <div className="r-main flex flex-col gap-3">
            <Skeleton shape="line" width="92%" />
            <Skeleton shape="line" width="60%" />
            <div className="flex gap-2">
              <Skeleton width={52} height={22} />
              <Skeleton width={60} height={22} />
              <Skeleton width={44} height={22} />
            </div>
            <Skeleton shape="line" width="70%" />
          </div>
          <div className="actions">
            <Skeleton width={96} height={44} />
          </div>
        </div>
      ))}
    </div>
  );
}

interface SearchingProps {
  /** What the field shows. */
  q: string;
  /** Known once the server answered (`complete: false`). */
  indexers: IndexerReport[];
  queries: SearchQuery[];
}

/**
 * Before the first answer, and while nothing has arrived: Noir's eyes and
 * "Searching 3 indexers…", each indexer's progress, and skeleton cards.
 * Nothing shows for the first 150 ms. Only the heading is announced, not
 * every tick of the timers.
 */
export function SearchingState({ q, indexers, queries }: SearchingProps) {
  const title = searchingTitle(indexers);
  return (
    <div className="ts-searching skel-delay">
      <div className="flex items-center gap-3">
        <span className="ts-eyes">
          <CatEyes width={44} />
        </span>
        <div className="flex min-w-0 flex-col">
          <strong className="font-semibold text-fg-1" role="status">
            {title}
          </strong>
          <span className="t-small text-fg-3">{searchingDetail(q, queries)}</span>
        </div>
      </div>
      {indexers.length > 0 && <IndexerList indexers={indexers} />}
      <ReleaseSkeletons />
    </div>
  );
}

/** Some indexers timed out or failed: what answered, who didn't, and Retry (`refresh=1`). */
export function PartialBanner({ indexers, retrying, onRetry }: { indexers: IndexerReport[]; retrying: boolean; onRetry: () => void }) {
  const problem = partialProblem(indexers);
  if (!problem) return null;
  return (
    <Banner
      tone={problem.none ? 'error' : 'warning'}
      role={problem.none ? 'alert' : 'status'}
      title={problem.title}
      action={
        <Button size="sm" loading={retrying} onClick={onRetry}>
          Retry
        </Button>
      }
    >
      {problem.body}
    </Banner>
  );
}

/** The whole search failed (Jackett off or down, TMDB down): what happened, and Retry when it can help. */
export function SearchErrorBanner({ problem, retrying, onRetry }: { problem: SearchProblem; retrying: boolean; onRetry: () => void }) {
  return (
    <Banner
      tone="error"
      role="alert"
      title={problem.title}
      action={
        problem.retry ? (
          <Button size="sm" loading={retrying} onClick={onRetry}>
            Retry
          </Button>
        ) : undefined
      }
    >
      {problem.body}
    </Banner>
  );
}

/** "10 episodes are missing · A pack gets 29–38 in one go." with a switch to season packs. */
export function PackBanner({ missing, absolute, onSeasonPacks }: { missing: SearchMissingRun; absolute: boolean; onSeasonPacks: () => void }) {
  const { title, body } = missingCopy(missing, absolute);
  return (
    <Banner
      tone="accent"
      icon={Layers}
      title={title}
      action={
        <Button size="sm" onClick={onSeasonPacks}>
          Season packs
        </Button>
      }
    >
      {body}
    </Banner>
  );
}

interface NoResultsProps {
  q: string;
  indexers: number;
  suggestions: NoResultsSuggestions;
  onSearch: (q: string) => void;
  onScope: (scope: SearchScope) => void;
  /** Shown only while the HDRezka tab is there. */
  onTryHDRezka?: () => void;
}

/** Nothing in scope: say so, then ways to broaden (a shorter query, a wider scope, the original title, HDRezka). */
export function NoResults({ q, indexers, suggestions, onSearch, onScope, onTryHDRezka }: NoResultsProps) {
  const { shorter, wider, original } = suggestions;
  return (
    <EmptyState
      cat="peek"
      title={noResultsTitle(q)}
      actions={
        <div className="flex w-[340px] max-w-full flex-col gap-2">
          {shorter && (
            <Button block onClick={() => onSearch(shorter)}>
              <Search size={17} aria-hidden="true" /> Try “{shorter}”
            </Button>
          )}
          {wider && (
            <Button block onClick={() => onScope(wider.scope)}>
              <Layers size={17} aria-hidden="true" /> {wider.label}
            </Button>
          )}
          {original && (
            <Button variant="ghost" block onClick={() => onSearch(original.q)}>
              {original.label}
            </Button>
          )}
          {onTryHDRezka && (
            <Button variant="ghost" block onClick={onTryHDRezka}>
              <MonitorPlay size={17} aria-hidden="true" /> Try HDRezka
            </Button>
          )}
        </div>
      }
    >
      {noResultsBody(indexers)}
    </EmptyState>
  );
}

/**
 * Filters hide everything the scope found. A plain stand-in: #93 names the
 * filters, shows them as removable chips and keeps "Clear all".
 */
export function FilteredEmpty({ total, onClear }: { total: number; onClear: () => void }) {
  return (
    <EmptyState
      cat="blank"
      title={`Your filters hide all ${total} results`}
      actions={
        <Button onClick={onClear}>
          Clear all filters
        </Button>
      }
    />
  );
}
