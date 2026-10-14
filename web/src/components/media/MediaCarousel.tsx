import type { SearchResult } from '@/types/tmdb';
import { MediaCard, MediaCardSkeleton } from './MediaCard';

interface MediaCarouselProps {
  title: string;
  items: SearchResult[];
}

export function MediaCarousel({ title, items }: MediaCarouselProps) {
  if (items.length === 0) {
    return null;
  }

  return (
    <section className="mb-8 sm:mb-10 lg:mb-12">
      <h2 className="text-xl sm:text-2xl font-semibold text-white mb-4 sm:mb-6 px-4 sm:px-6 lg:px-12">
        {title}
      </h2>
      <div
        className="
          flex gap-3 sm:gap-4 lg:gap-6
          overflow-x-auto snap-x snap-mandatory
          px-4 sm:px-6 lg:px-12
          pb-4
          scrollbar-hide
        "
      >
        {items.map((item, index) => (
          <div key={item.id} className="snap-start">
            <MediaCard media={item} priority={index < 5} />
          </div>
        ))}
      </div>
    </section>
  );
}

export function MediaCarouselSkeleton({ title }: { title: string }) {
  return (
    <section className="mb-8 sm:mb-10 lg:mb-12">
      <h2 className="text-xl sm:text-2xl font-semibold text-white mb-4 sm:mb-6 px-4 sm:px-6 lg:px-12">
        {title}
      </h2>
      <div
        className="
          flex gap-3 sm:gap-4 lg:gap-6
          overflow-x-auto
          px-4 sm:px-6 lg:px-12
          pb-4
          scrollbar-hide
        "
      >
        {Array.from({ length: 7 }).map((_, index) => (
          <MediaCardSkeleton key={index} />
        ))}
      </div>
    </section>
  );
}
