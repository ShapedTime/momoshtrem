import { LoadingRegion, Skeleton, SkeletonText } from '@/components/ui';
import { ErrorView } from '@/components/system/ErrorView';
import { BackButton } from './BackButton';
import { isApiError } from '@/lib/api/client';

/**
 * The detail page while it loads: the same hero grid, panel and body boxes
 * as the real thing, and Back on phone straight away (the shell's phone bar
 * steps aside for hero pages).
 */
export function TitleDetailSkeleton() {
  return (
    <>
      <div className="hero detail-hero">
        <div className="phone-bar">
          <BackButton />
        </div>
        <LoadingRegion label="Loading the title">
          <div className="page detail-grid">
            <div className="poster-col">
              <Skeleton shape="poster" className="rounded-xl" />
            </div>
            <div className="detail-title">
              <Skeleton shape="line" width={80} />
              <Skeleton height={44} width="60%" />
              <Skeleton shape="line" width="45%" />
              <SkeletonText lines={3} />
            </div>
            <Skeleton height={196} className="rounded-xl" />
          </div>
        </LoadingRegion>
      </div>
      <div className="page detail-body skel-delay" aria-hidden="true">
        <div className="page-sections">
          <div className="flex flex-col gap-4">
            <Skeleton shape="line" width={64} />
            <div className="flex gap-3 overflow-hidden">
              {[0, 1, 2, 3, 4].map((i) => (
                <Skeleton key={i} shape="circle" width={72} height={72} className="shrink-0" />
              ))}
            </div>
          </div>
          <Skeleton shape="wide" className="lg:max-w-[720px]" />
        </div>
        <Skeleton height={240} className="rounded-lg" />
      </div>
    </>
  );
}

/** What went wrong, in words for a title page: TMDB down or not set up, else the server's message. */
function explain(error: unknown, noun: string): string {
  const fine = 'Your library and Infuse aren’t affected.';
  if (isApiError(error)) {
    if (error.code === 'tmdb_unavailable') return `TMDB isn’t answering and this ${noun} isn’t cached yet. Try again in a minute. ${fine}`;
    if (error.code === 'unavailable') return `TMDB isn’t set up on the server yet: add TMDB_API_KEY to its .env. ${fine}`;
    if (error.message) return `${error.message} ${fine}`;
  }
  return `Something went wrong while loading it. ${fine}`;
}

/** A title page that failed to load: what failed, what's fine, Try again. */
export function TitleLoadError({ error, noun, onRetry }: { error: unknown; noun: 'movie' | 'show'; onRetry: () => void }) {
  return <ErrorView title={`This ${noun} didn’t load`} message={explain(error, noun)} onRetry={onRetry} />;
}

/** A 404 from the title endpoint: TMDB doesn't know the id. */
export const isTitleNotFound = (error: unknown) => isApiError(error) && error.status === 404;
