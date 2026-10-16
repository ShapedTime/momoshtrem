import { Bookmark, History, Link2, Star } from 'lucide-react';
import { EmptyCat } from '@/components/brand';
import { Banner, Button, Panel } from '@/components/ui';
import { TRAKT_ENV } from '@/lib/settings/trakt';

const IMPORTS = [
  { icon: History, name: 'History', use: 'what you’ve watched, for Up next' },
  { icon: Star, name: 'Ratings', use: 'what you loved, for picks' },
  { icon: Bookmark, name: 'Watchlist', use: 'shown on For You' },
];

interface TraktOffProps {
  /** No Trakt app or SECRET_KEY on the server: say what to set instead of offering Connect. */
  unconfigured: boolean;
  connecting: boolean;
  onConnect: () => void;
}

/** Not connected (screens/settings/trakt-off): Noir peeking, what's read, and Connect. */
export function TraktOff({ unconfigured, connecting, onConnect }: TraktOffProps) {
  return (
    <Panel as="section" aria-labelledby="trakt-off-h" className="set-card">
      <div className="set-trakt-off">
        <div className="set-trakt-art">
          <EmptyCat variant="peek" size={120} />
        </div>
        <div className="flex min-w-0 flex-col gap-3">
          <h3 id="trakt-off-h" className="t-title-2" tabIndex={-1} data-screen-focus>
            Connect Trakt
          </h3>
          <p className="text-fg-2">
            momoshtrem reads your watch history, ratings and watchlist to build Up next and picks that fit you. It’s read-only: nothing is ever
            posted to Trakt.
          </p>
          <ul className="set-imports" aria-label="What momoshtrem reads">
            {IMPORTS.map(({ icon: Icon, name, use }) => (
              <li key={name}>
                <Icon size={16} aria-hidden="true" />
                {name}
                <span>{use}</span>
              </li>
            ))}
          </ul>
          {unconfigured ? (
            <Banner title="Trakt isn’t set up on this server">
              Create an app at trakt.tv/oauth/applications, then set{' '}
              {TRAKT_ENV.map((name, index) => (
                <span key={name}>
                  <code className="t-mono text-fg-1">{name}</code>
                  {index < TRAKT_ENV.length - 2 ? ', ' : index === TRAKT_ENV.length - 2 ? ' and ' : ''}
                </span>
              ))}{' '}
              in the server’s .env and restart momoshtrem.
            </Banner>
          ) : (
            <div className="flex flex-wrap items-center gap-3">
              <Button variant="primary" loading={connecting} onClick={onConnect}>
                <Link2 size={18} aria-hidden="true" /> Connect Trakt
              </Button>
              <span className="t-small text-fg-3">You’ll get a code to enter on trakt.tv. No password here.</span>
            </div>
          )}
        </div>
      </div>
    </Panel>
  );
}
