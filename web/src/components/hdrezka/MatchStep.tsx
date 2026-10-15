import { Check, ExternalLink, Search, Sparkle, TriangleAlert } from 'lucide-react';
import { Banner, Button, EmptyState, LoadingRegion, Skeleton } from '@/components/ui';
import type { HDRezkaCandidate } from '@/lib/api/contract';
import { hdrezkaProblem, shorterQuery } from '@/lib/hdrezka/copy';
import { bestCandidate, candidateFacts, type MatchTarget, matchReasons, weakMatchNote } from '@/lib/hdrezka/pick';
import { cx } from '@/lib/utils/cx';

const label = (candidate: Pick<HDRezkaCandidate, 'title' | 'year'>) => (candidate.year ? `${candidate.title} (${candidate.year})` : candidate.title);

/** "Open on HDRezka": an icon on phone, with its word on desktop (the magnet slot of a release card). */
export function OpenOnHDRezka({ url, name }: { url: string; name: string }) {
  return (
    <a className="btn btn-ghost hd-open" href={url} target="_blank" rel="noopener noreferrer" aria-label={`Open ${name} on HDRezka`}>
      <ExternalLink size={18} aria-hidden="true" />
      <span className="lbl" aria-hidden="true">
        HDRezka
      </span>
    </a>
  );
}

interface CandidateCardProps {
  candidate: HDRezkaCandidate;
  /** The best match: amber outline, its reasons, and the list's only primary button. */
  best: boolean;
  target: MatchTarget;
  onUse: () => void;
}

/** One HDRezka hit, styled like a torrent result (`.release`). Weak hits look weaker and say so. */
export function CandidateCard({ candidate, best, target, onUse }: CandidateCardProps) {
  const weak = candidate.match === 'weak';
  const name = label(candidate);
  const original = candidate.original_title && candidate.original_title !== candidate.title ? candidate.original_title : null;
  return (
    <article className={cx('release hd-candidate', best && 'is-best', weak && 'is-weak')} aria-label={name}>
      <div className="r-main flex min-w-0 flex-col gap-2">
        {best && (
          <p className="best-head">
            <Sparkle size={15} aria-hidden="true" /> Best match
          </p>
        )}
        <div className="flex min-w-0 flex-col">
          <h3 className="hd-name" lang="ru">
            {candidate.title}
          </h3>
          {original && <p className="t-small text-fg-2">{original}</p>}
        </div>
        <p className={best ? 'why' : 'facts'}>
          {(best ? matchReasons(candidate, target) : candidateFacts(candidate)).map((fact) => (
            <span key={fact}>{fact}</span>
          ))}
        </p>
        {weak && (
          <p className="warnline">
            <TriangleAlert size={15} aria-hidden="true" /> {weakMatchNote(target)}
          </p>
        )}
      </div>
      <div className="actions">
        <Button variant={best ? 'primary' : 'secondary'} onClick={onUse} aria-label={`Use ${name}`}>
          <Check size={18} aria-hidden="true" /> Use
        </Button>
        <span className="spacer" />
        <OpenOnHDRezka url={candidate.url} name={name} />
      </div>
    </article>
  );
}

/** Three cards the size of real ones, after 150 ms (HANDOFF §4.3). */
export function CandidateSkeletons() {
  return (
    <LoadingRegion label="Searching HDRezka" className="flex flex-col gap-3">
      {[0, 1, 2].map((i) => (
        <div key={i} className="release" aria-hidden="true">
          <div className="flex flex-col gap-3">
            <Skeleton shape="line" width="62%" />
            <Skeleton shape="line" width="40%" />
            <div className="flex gap-2">
              <Skeleton width={44} height={20} />
              <Skeleton width={40} height={20} />
              <Skeleton width={72} height={20} />
            </div>
          </div>
          <Skeleton width={96} height={44} />
        </div>
      ))}
    </LoadingRegion>
  );
}

interface MatchStepProps {
  target: MatchTarget;
  /** What was searched, for "No match for …". */
  query: string;
  status: 'pending' | 'error' | 'success';
  items: HDRezkaCandidate[];
  error: unknown;
  onRetry: () => void;
  onUse: (candidate: HDRezkaCandidate) => void;
  /** Search again with another query. */
  onSearch: (query: string) => void;
  /** Focus the field for a pasted link. */
  onPasteLink: () => void;
  onTryTorrents: () => void;
}

/**
 * Step 1: which HDRezka title it is. Like torrent search (HANDOFF §5.2):
 * skeletons while searching, the best match first, suggestions when nothing
 * matched, and a banner with Retry when HDRezka fails.
 */
export function MatchStep({ target, query, status, items, error, onRetry, onUse, onSearch, onPasteLink, onTryTorrents }: MatchStepProps) {
  if (status === 'pending') return <CandidateSkeletons />;

  if (status === 'error') {
    const problem = hdrezkaProblem(error);
    return (
      <div className="flex flex-col items-start gap-3">
        <Banner
          tone="error"
          role="alert"
          title={problem.title}
          className="self-stretch"
          action={
            problem.retry ? (
              <Button size="sm" onClick={onRetry}>
                Retry
              </Button>
            ) : undefined
          }
        >
          {problem.body}
        </Banner>
        <Button variant="ghost" size="sm" onClick={onTryTorrents}>
          <Search size={16} aria-hidden="true" /> Find a torrent instead
        </Button>
      </div>
    );
  }

  if (items.length === 0) {
    const shorter = shorterQuery(query);
    // The original title first: HDRezka keeps it next to the Russian one
    const original = [target.original_title, target.title].find((name) => name && name !== query && name !== shorter) ?? null;
    return (
      <EmptyState
        cat="peek"
        title={`No match for “${query}”`}
        actions={
          <div className="flex w-[340px] max-w-full flex-col gap-2">
            {shorter && (
              <Button block onClick={() => onSearch(shorter)}>
                <Search size={17} aria-hidden="true" /> Try “{shorter}”
              </Button>
            )}
            {original && (
              <Button block onClick={() => onSearch(original)}>
                <Search size={17} aria-hidden="true" /> Search “{original}”
              </Button>
            )}
            <Button block onClick={onPasteLink}>
              Paste an HDRezka link
            </Button>
            <Button variant="ghost" block onClick={onTryTorrents}>
              Try torrents
            </Button>
          </div>
        }
      >
        HDRezka lists titles under their Russian names. A shorter name, the original title or the page’s link usually finds it.
      </EmptyState>
    );
  }

  const best = bestCandidate(items);
  const rest = items.filter((item) => item !== best);
  return (
    <div className="flex flex-col gap-3">
      <p className="t-small text-fg-3">
        {items.length === 1 ? '1 match' : `${items.length} matches`} on HDRezka for “{query}”
      </p>
      {best && <CandidateCard candidate={best} best target={target} onUse={() => onUse(best)} />}
      {rest.length > 0 && (
        <>
          <h3 className="t-micro text-fg-3 mt-2">{best ? 'Other matches' : 'Matches'}</h3>
          {rest.map((item) => (
            <CandidateCard key={item.url} candidate={item} best={false} target={target} onUse={() => onUse(item)} />
          ))}
        </>
      )}
    </div>
  );
}
