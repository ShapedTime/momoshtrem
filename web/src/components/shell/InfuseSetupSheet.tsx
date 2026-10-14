'use client';

import { useQuery } from '@tanstack/react-query';
import { Banner, CopyField, FilePath, Skeleton } from '@/components/ui';
import { statusQuery } from '@/lib/api/endpoints/system';

/** "/Movies/Dune (2024)/Dune (2024).mkv" → "Movies › Dune (2024)": where Infuse lists it. */
function libraryTrail(path: string): string {
  const parts = path.split('/').filter(Boolean);
  const folders = /\.[a-z0-9]{2,4}$/i.test(parts.at(-1) ?? '') ? parts.slice(0, -1) : parts;
  return folders.join(' › ');
}

/**
 * `?sheet=infuse-setup` params for one title: its WebDAV path, and the URL
 * the server built for it (`webdav_url`), which is what Copy copies.
 */
export function infuseSetupParams({ path, url }: { path: string | null; url: string | null }): Record<string, string> {
  return { ...(path ? { path } : {}), ...(path && url ? { url } : {}) };
}

/**
 * "Play it in Infuse": the three steps and the WebDAV address (from /status).
 * With a title's `path`, the last step says where that title is and offers
 * its `url` (or the path, when there's no URL).
 */
export function InfuseSetupSheet({ path, url }: { path?: string | null; url?: string | null }) {
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
        {path ? (
          <div className="flex min-w-0 flex-col gap-2">
            <p className="font-semibold">Find it under Library</p>
            <p className="t-small text-fg-2">
              {libraryTrail(path)}. It appears after Infuse’s next library refresh (pull down to refresh).
            </p>
            <CopyField
              value={url ?? path}
              label="WebDAV path of this title"
              hint="Open it in Infuse › Add files › WebDAV"
            >
              <FilePath path={path} />
            </CopyField>
          </div>
        ) : (
          <div className="flex flex-col gap-0.5">
            <p className="font-semibold">Find it under Library</p>
            <p className="t-small text-fg-2">
              Infuse sorts Movies and Shows by itself. New episodes appear after its next library refresh (pull down to
              refresh).
            </p>
          </div>
        )}
      </li>
    </ol>
  );
}
