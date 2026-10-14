import { PosterCard, Rail, RailItem, StatusPill } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import type { AvailabilityStage, SimilarTitle } from '@/lib/api/contract';
import type { StatusKey } from '@/lib/status';

/** Library pills on the cards; titles outside the library get none. Short labels for narrow cards. */
const PILL: Record<AvailabilityStage, { status: StatusKey; label?: string } | null> = {
  none: null,
  needs_torrent: { status: 'needs_torrent', label: 'No torrent' },
  downloading: { status: 'downloading' },
  ready: { status: 'ready' },
};

export const titleHref = (mediaType: SimilarTitle['media_type'], tmdbId: number) => `/${mediaType === 'movie' ? 'movie' : 'tv'}/${tmdbId}`;

/** "More like this": TMDB recommendations with what's already in your library marked. */
export function MoreLikeThis({ items }: { items: SimilarTitle[] }) {
  if (!items.length) return null;
  return (
    <Rail title="More like this">
      {items.map((item) => {
        const pill = item.library ? PILL[item.library.availability_stage] : null;
        const subtitle = [item.year, item.media_type === 'tv' ? 'Series' : null].filter(Boolean).join(' · ');
        return (
          <RailItem key={`${item.media_type}-${item.tmdb_id}`}>
            <PosterCard
              href={titleHref(item.media_type, item.tmdb_id)}
              title={item.title}
              subtitle={subtitle || undefined}
              posterSrc={buildImageUrl(item.poster_path, 'poster', 'medium')}
              badge={pill && <StatusPill status={pill.status} label={pill.label} onArt />}
            />
          </RailItem>
        );
      })}
    </Rail>
  );
}
