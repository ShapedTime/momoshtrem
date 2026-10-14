'use client';

import { Search } from 'lucide-react';
import { EmptyCat } from '@/components/brand';
import { Button, ButtonLink } from '@/components/ui';
import { ShellContext } from '@/components/shell/ShellContext';
import { usePathname } from 'next/navigation';
import { useContext } from 'react';

/**
 * 404 (screens/system/notfound): Noir peeking over a ledge. Inside the shell
 * "Search" opens the palette; outside it, it goes to the search page.
 */
export function NotFoundView() {
  const shell = useContext(ShellContext);
  const pathname = usePathname();
  const path = pathname && pathname !== '/' ? pathname : undefined;
  return (
    <div className="page flex min-h-[70vh] flex-col items-center justify-center gap-5 py-12 text-center">
      <EmptyCat variant="peek" size={200} className="text-surface-4" />
      <p className="t-mono tracking-[0.1em] text-fg-3">404</p>
      <h1 className="t-title-1">Nothing here but the cat</h1>
      <p className="max-w-[46ch] text-fg-2">
        {path && (
          <>
            <span className="t-mono text-fg-1">{path}</span> doesn’t exist.{' '}
          </>
        )}
        The link may be from an older version, or the title was removed from your library.
      </p>
      <div className="flex flex-wrap justify-center gap-2">
        <ButtonLink href="/" variant="primary">
          Go to For You
        </ButtonLink>
        {shell ? (
          <Button onClick={() => shell.openPalette()}>
            <Search size={16} aria-hidden="true" /> Search
          </Button>
        ) : (
          <ButtonLink href="/search">
            <Search size={16} aria-hidden="true" /> Search
          </ButtonLink>
        )}
      </div>
    </div>
  );
}
