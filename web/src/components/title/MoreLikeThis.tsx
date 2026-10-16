import { libraryPill } from '@/components/library/LibraryPill';
import { PosterCard, Rail, RailItem } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import type { SimilarTitle } from '@/lib/api/contract';
import { titleHref } from '@/lib/title/href';

export { titleHref };

/** "More like this": TMDB recommendations with what's already in your library marked. */
export function MoreLikeThis({ items }: { items: SimilarTitle[] }) {
  if (!items.length) return null;
  return (
    <Rail title="More like this">
      {items.map((item) => {
        const subtitle = [item.year, item.media_type === 'tv' ? 'Series' : null].filter(Boolean).join(' · ');
        return (
          <RailItem key={`${item.media_type}-${item.tmdb_id}`}>
            <PosterCard
              href={titleHref(item.media_type, item.tmdb_id)}
              title={item.title}
              subtitle={subtitle || undefined}
              posterSrc={buildImageUrl(item.poster_path, 'poster', 'medium')}
              badge={libraryPill(item.library)}
            />
          </RailItem>
        );
      })}
    </Rail>
  );
}
