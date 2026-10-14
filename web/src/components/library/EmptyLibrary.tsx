import { Search } from 'lucide-react';
import { ButtonLink, EmptyState } from '@/components/ui';

/** Library with nothing in it yet (screens/system gallery: "Your library is empty"). */
export function EmptyLibrary() {
  return (
    <EmptyState
      cat="sit"
      title="Your library is empty"
      headingLevel="h2"
      actions={
        <>
          <ButtonLink href="/movies" variant="primary">
            Browse Discover
          </ButtonLink>
          <ButtonLink href="/search" variant="ghost">
            <Search size={16} aria-hidden="true" /> Search
          </ButtonLink>
        </>
      }
    >
      Add a movie or show and it lands here, then in Infuse once it’s downloaded.
    </EmptyState>
  );
}
