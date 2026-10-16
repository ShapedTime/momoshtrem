'use client';

import { Button, Loader, Panel, Steps } from '@/components/ui';
import { useNow } from '@/hooks/useNow';
import type { TraktConnect } from '@/lib/api/contract';
import { secondsLeft, spokenCode, spokenTimeLeft, waitingLine } from '@/lib/settings/trakt';

interface TraktWaitingProps {
  code: TraktConnect;
  cancelling: boolean;
  onCancel: () => void;
  /** Back to the code: the flag is local, the code is still live. */
  onShowCode: () => void;
}

const STEPS = [
  { label: 'Code shown', state: 'done' as const },
  { label: 'Approval on trakt.tv', state: 'active' as const },
  { label: 'First sync', state: 'pending' as const },
];

/** Waiting for the approval (screens/settings/trakt-waiting): Noir's eyes while the backend checks Trakt. */
export function TraktWaiting({ code, cancelling, onCancel, onShowCode }: TraktWaitingProps) {
  const now = useNow(1000);
  const left = secondsLeft(code.expires_at, now);

  return (
    <Panel as="section" aria-label="Waiting for Trakt" className="set-card">
      <div className="set-waiting" tabIndex={-1} data-screen-focus>
        <Loader
          title="Waiting for you to approve on Trakt…"
          detail={
            <>
              {/* Screen readers hear the countdown once a minute, not every second */}
              <span aria-hidden="true">{waitingLine(code.interval, code.user_code, left)}</span>
              <span className="sr-only">
                Checking every {code.interval} seconds. Code {spokenCode(code.user_code)} expires {spokenTimeLeft(left)}.
              </span>
            </>
          }
        />
        <Steps inline steps={STEPS} className="set-wait-steps" />
      </div>
      <div className="set-card-foot">
        <Button variant="ghost" loading={cancelling} onClick={onCancel}>
          Cancel
        </Button>
        <Button variant="ghost" onClick={onShowCode}>
          Show the code again
        </Button>
      </div>
    </Panel>
  );
}
