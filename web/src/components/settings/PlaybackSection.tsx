'use client';

import { useQuery } from '@tanstack/react-query';
import { Banner, CopyField, Panel, Skeleton } from '@/components/ui';
import { statusQuery } from '@/lib/api/endpoints/system';

const STEPS = [
  { title: 'On Apple TV, open Infuse', detail: 'Settings › Shares › Add share › WebDAV' },
  { title: 'Paste the address above', detail: 'Name it “momoshtrem”. Use the WebDAV username and password shown above, if there are any.' },
  {
    title: 'Add it to the Library',
    detail: 'Infuse sorts Movies and Shows by itself. New downloads appear after its next refresh; pull down to refresh sooner.',
  },
];

/** Infuse & WebDAV (screens/settings/playback). */
export function PlaybackSection() {
  const { data, isPending, isError } = useQuery(statusQuery());

  return (
    <div className="flex flex-col gap-5">
      <Panel as="section" aria-labelledby="webdav-h" className="flex flex-col gap-3 p-4 sm:p-5">
        <h3 id="webdav-h" className="t-title-3">
          WebDAV address
        </h3>
        {isPending ? (
          <Skeleton height={44} />
        ) : isError || !data ? (
          <Banner tone="warning" title="Couldn’t load the address">
            It’s WEBDAV_PUBLIC_URL in the server’s .env.
          </Banner>
        ) : (
          <>
            <CopyField value={data.webdav.url} label="WebDAV address" hint="Paste it into Infuse › Add share › WebDAV" />
            <p className="t-small text-fg-3">
              {data.webdav.auth_enabled && data.webdav.username ? (
                <>
                  Username <span className="font-semibold text-fg-1">{data.webdav.username}</span> · the password is WEBDAV_PASSWORD
                  on the server.
                </>
              ) : (
                'No username or password: anyone on your network can browse it. Set WEBDAV_AUTH_ENABLED to require one.'
              )}
            </p>
          </>
        )}
      </Panel>

      <Panel as="section" aria-labelledby="infuse-h" className="flex flex-col gap-4 p-4 sm:p-5">
        <h3 id="infuse-h" className="t-title-3">
          Set up Infuse (once)
        </h3>
        <ol className="howto-steps">
          {STEPS.map((step, index) => (
            <li key={step.title}>
              <span className="n">{index + 1}</span>
              <div className="flex flex-col gap-0.5">
                <p className="font-semibold">{step.title}</p>
                <p className="t-small text-fg-2">{step.detail}</p>
              </div>
            </li>
          ))}
        </ol>
      </Panel>
    </div>
  );
}
