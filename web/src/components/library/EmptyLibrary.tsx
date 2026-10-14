import Link from 'next/link';
import { Button } from '@/components/ui/Button';

function LibraryIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="64"
      height="64"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="text-text-muted"
      aria-hidden="true"
    >
      <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" />
      <path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z" />
      <line x1="12" y1="6" x2="12" y2="10" />
      <line x1="10" y1="8" x2="14" y2="8" />
    </svg>
  );
}

export function EmptyLibrary() {
  return (
    <div className="flex flex-col items-center justify-center py-16 px-4 text-center">
      <LibraryIcon />

      <h2 className="mt-6 text-xl font-semibold text-white">
        Your library is empty
      </h2>

      <p className="mt-2 text-text-secondary max-w-md">
        Add movies and TV shows to your library to keep track of what you want to watch
        and easily find torrents for them.
      </p>

      <div className="mt-8 flex flex-col sm:flex-row gap-4">
        <Link href="/">
          <Button variant="primary" size="lg">
            Browse Movies
          </Button>
        </Link>
        <Link href="/search">
          <Button variant="secondary" size="lg">
            Search
          </Button>
        </Link>
      </div>
    </div>
  );
}
