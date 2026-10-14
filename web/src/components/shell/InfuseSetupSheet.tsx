'use client';

import { useQuery } from '@tanstack/react-query';
import { Banner, CopyField, Skeleton } from '@/components/ui';
import { statusQuery } from '@/lib/api/endpoints/system';

/** "Play it in Infuse": the three steps and the WebDAV address (from /status). */
export function InfuseSetupSheet() {
  const { data, isPending, isError } = useQuery(statusQuery());
  return (
    <ol className="howto-steps">
      <li>
        <span className="n">1</span>
        <div className="flex flex-col gap-0.5">
          <p className="font-semibold">Open Infuse on Apple TV</p>
          <p className="t-small text-fg-2">Settings › Shares › Add share › WebDAV</p>
        </div>
      </li>
      <li>
        <span className="n">2</span>
        <div className="flex min-w-0 flex-col gap-2">
          <p className="font-semibold">Use this address</p>
          {isPending ? (
            <Skeleton height={44} />
          ) : isError || !data ? (
            <Banner tone="warning" title="Couldn’t load the address">
              It’s WEBDAV_PUBLIC_URL in the server’s .env.
            </Banner>
          ) : (
            <>
              <CopyField value={data.webdav.url} label="WebDAV address" hint="Paste it into Infuse › Add share › WebDAV" />
              <p className="t-small text-fg-2">
                {data.webdav.auth_enabled && data.webdav.username ? (
                  <>
                    Username <span className="font-semibold text-fg-1">{data.webdav.username}</span>, with the WebDAV password
                    from the server’s settings.
                  </>
                ) : (
                  'No username or password needed.'
                )}
              </p>
            </>
          )}
        </div>
      </li>
      <li>
        <span className="n">3</span>
        <div className="flex flex-col gap-0.5">
          <p className="font-semibold">Find it under Library</p>
          <p className="t-small text-fg-2">
            Infuse sorts Movies and Shows by itself. New episodes appear after its next library refresh (pull down to
            refresh).
          </p>
        </div>
      </li>
    </ol>
  );
}
