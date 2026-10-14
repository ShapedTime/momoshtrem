import Image from 'next/image';
import type { CastMember } from '@/types/tmdb';
import { buildImageUrl } from '@/config/tmdb';

interface CastCarouselProps {
  cast: CastMember[];
  maxItems?: number;
}

function CastCard({ member }: { member: CastMember }) {
  const profileUrl = buildImageUrl(member.profilePath, 'profile', 'medium');

  return (
    <div className="flex-shrink-0 snap-start w-[100px] sm:w-[120px] lg:w-[140px]">
      {/* Profile Image Container */}
      <div className="relative aspect-[2/3] rounded-md overflow-hidden bg-bg-elevated mb-2">
        {profileUrl ? (
          <Image
            src={profileUrl}
            alt={member.name}
            fill
            sizes="(max-width: 640px) 100px, (max-width: 1024px) 120px, 140px"
            className="object-cover"
          />
        ) : (
          <div className="absolute inset-0 flex items-center justify-center bg-bg-hover text-text-muted">
            <span className="text-2xl font-semibold">
              {member.name.charAt(0).toUpperCase()}
            </span>
          </div>
        )}
      </div>

      {/* Name and Character */}
      <p className="text-sm font-medium text-white line-clamp-1">{member.name}</p>
      <p className="text-xs text-text-secondary line-clamp-1">{member.character}</p>
    </div>
  );
}

export function CastCarousel({ cast, maxItems = 10 }: CastCarouselProps) {
  const displayCast = cast
    .slice()
    .sort((a, b) => a.order - b.order)
    .slice(0, maxItems);

  if (displayCast.length === 0) {
    return null;
  }

  return (
    <section className="mb-8 sm:mb-10 lg:mb-12">
      <h2 className="text-xl sm:text-2xl font-semibold text-white mb-4 sm:mb-6 px-4 sm:px-6 lg:px-12">
        Cast
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
        {displayCast.map((member) => (
          <CastCard key={member.id} member={member} />
        ))}
      </div>
    </section>
  );
}

export function CastCarouselSkeleton() {
  return (
    <section className="mb-8 sm:mb-10 lg:mb-12">
      <div className="h-7 sm:h-8 w-16 bg-bg-hover rounded mb-4 sm:mb-6 mx-4 sm:mx-6 lg:mx-12 animate-pulse motion-reduce:animate-none" />
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
          <div
            key={index}
            className="flex-shrink-0 w-[100px] sm:w-[120px] lg:w-[140px]"
          >
            <div className="aspect-[2/3] rounded-md bg-bg-hover animate-pulse motion-reduce:animate-none mb-2" />
            <div className="h-4 w-3/4 bg-bg-hover rounded animate-pulse motion-reduce:animate-none mb-1" />
            <div className="h-3 w-1/2 bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
          </div>
        ))}
      </div>
    </section>
  );
}
