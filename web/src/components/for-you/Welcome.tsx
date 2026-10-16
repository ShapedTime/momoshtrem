'use client';

import { useId } from 'react';
import { Link2 } from 'lucide-react';
import { EmptyCat } from '@/components/brand';
import { useShell } from '@/components/shell/ShellContext';
import { Button, ButtonLink } from '@/components/ui';
import { useSheet } from '@/lib/sheets/useSheet';

export const TRAKT_SETTINGS_HREF = '/settings?section=trakt';

/** Without Trakt: what connecting brings, and a way to browse instead (js/screens/home.js `connectCard`). */
export function ConnectCard() {
  const headingId = useId();
  return (
    <section className="connect-card panel" aria-labelledby={headingId}>
      <div className="connect-art">
        <EmptyCat variant="peek" size={132} />
      </div>
      <div className="flex min-w-0 flex-col gap-3">
        <h2 className="t-title-2" id={headingId}>
          Make this page yours
        </h2>
        <p className="text-fg-2">
          Connect Trakt and momoshtrem learns from what you’ve watched. You get Up next for your shows and picks with a reason
          for each. It’s read-only: nothing is posted to Trakt.
        </p>
        <div className="flex flex-wrap gap-2">
          <ButtonLink href={TRAKT_SETTINGS_HREF} variant="primary">
            <Link2 size={18} aria-hidden="true" /> Connect Trakt
          </ButtonLink>
          <ButtonLink href="/discover" variant="ghost">
            Browse instead
          </ButtonLink>
        </div>
      </div>
    </section>
  );
}

/** No Trakt and nothing in the library yet: Noir says hello, with the three steps that fill this page. */
export function Welcome() {
  const { openPalette } = useShell();
  const sheets = useSheet();
  return (
    <section className="welcome page" aria-labelledby="welcome-title">
      <div className="welcome-art">
        <EmptyCat variant="sit" size={120} />
      </div>
      <h1 className="t-display" id="welcome-title">
        Evening. I’m Noir.
      </h1>
      <p className="t-body-l max-w-[52ch] text-fg-2">
        I find things to watch, get them ready, and hand them to Infuse on your Apple TV. Three steps and this page fills up.
      </p>
      <ol className="welcome-steps">
        <li>
          <span className="n" aria-hidden="true">
            1
          </span>
          <div>
            <strong>Connect Trakt</strong>
            <p className="t-small text-fg-2">Your history powers Up next and personal picks.</p>
          </div>
          <ButtonLink href={TRAKT_SETTINGS_HREF} variant="primary" size="sm" aria-label="Connect Trakt">
            Connect
          </ButtonLink>
        </li>
        <li>
          <span className="n" aria-hidden="true">
            2
          </span>
          <div>
            <strong>Add something to your library</strong>
            <p className="t-small text-fg-2">Search for a title or browse Discover.</p>
          </div>
          <Button size="sm" onClick={openPalette} aria-label="Search for a title">
            Search
          </Button>
        </li>
        <li>
          <span className="n" aria-hidden="true">
            3
          </span>
          <div>
            <strong>Point Infuse at momoshtrem</strong>
            <p className="t-small text-fg-2">One WebDAV share, set up once.</p>
          </div>
          <Button size="sm" onClick={() => sheets.open('infuse-setup')} aria-label="Show me how to set up Infuse">
            Show me
          </Button>
        </li>
      </ol>
    </section>
  );
}
