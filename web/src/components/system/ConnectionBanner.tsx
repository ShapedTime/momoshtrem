'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useSyncExternalStore } from 'react';
import { useQuery } from '@tanstack/react-query';
import { RefreshCw, WifiOff } from 'lucide-react';
import { Banner, Button } from '@/components/ui';
import { isApiError } from '@/lib/api/client';
import { statusQuery } from '@/lib/api/endpoints/system';

function useOnline(): boolean {
  return useSyncExternalStore(
    (onChange) => {
      window.addEventListener('online', onChange);
      window.addEventListener('offline', onChange);
      return () => {
        window.removeEventListener('online', onChange);
        window.removeEventListener('offline', onChange);
      };
    },
    () => navigator.onLine,
    () => true
  );
}

const UNREACHABLE = new Set(['backend_unreachable', 'timeout']);

/**
 * Top-of-page banner when this device is offline or momoshtrem's API isn't
 * answering (screens/system: "Backend offline (banner)"), with Retry and a
 * link to the full offline page.
 */
export function ConnectionBanner() {
  const online = useOnline();
  const pathname = usePathname();
  const status = useQuery(statusQuery());
  const backendDown = status.isError && isApiError(status.error) && UNREACHABLE.has(status.error.code);

  if (!online) {
    return (
      <div className="page pt-4">
        <Banner tone="warning" role="status" icon={WifiOff} title="You’re offline">
          Showing what was already loaded. Infuse on your Apple TV doesn’t need this page to play.
        </Banner>
      </div>
    );
  }

  if (!backendDown) return null;
  return (
    <div className="page pt-4">
      <Banner
        tone="error"
        role="alert"
        icon={WifiOff}
        title="Can’t reach momoshtrem"
        action={
          <>
            <Button size="sm" loading={status.isFetching} onClick={() => status.refetch()}>
              <RefreshCw size={15} aria-hidden="true" /> Retry
            </Button>
            <Link href={`/offline?from=${encodeURIComponent(pathname)}`} className="btn btn-ghost btn-sm">
              Details
            </Link>
          </>
        }
      >
        Search, adding and Activity wait until the server answers. Nothing you’ve added is lost.
      </Banner>
    </div>
  );
}
