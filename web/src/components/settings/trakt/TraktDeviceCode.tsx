'use client';

import { Check, Copy, ExternalLink } from 'lucide-react';
import { CatEyes } from '@/components/brand';
import { Button, Panel, Progress, useCopy } from '@/components/ui';
import { useNow } from '@/hooks/useNow';
import type { TraktConnect } from '@/lib/api/contract';
import { displayCode, displayUrl, expiryPct, formatCountdown, secondsLeft, spokenCode, spokenTimeLeft } from '@/lib/settings/trakt';

interface TraktDeviceCodeProps {
  code: TraktConnect;
  cancelling: boolean;
  onCancel: () => void;
  /** "I've approved it": the waiting screen (the backend notices the approval on its own). */
  onApproved: () => void;
}

/** The device code (screens/settings/trakt-code): the code to type on trakt.tv, its countdown, and the two steps. */
export function TraktDeviceCode({ code, cancelling, onCancel, onApproved }: TraktDeviceCodeProps) {
  const copy = useCopy();
  const now = useNow(1000);
  const left = secondsLeft(code.expires_at, now);
  const site = displayUrl(code.verification_url);

  return (
    <Panel as="section" aria-label="Connect with a code" className="set-card set-card--code">
      <div className="set-device">
        <div className="set-device-code">
          <span className="t-micro text-fg-3" aria-hidden="true">
            Your code
          </span>
          <p className="code" tabIndex={-1} data-screen-focus>
            <span aria-hidden="true">{displayCode(code.user_code)}</span>
            <span className="sr-only">Your code: {spokenCode(code.user_code)}</span>
          </p>
          <Button size="sm" onClick={() => copy(code.user_code, `Paste it at ${site}.`)}>
            <Copy size={15} aria-hidden="true" /> Copy code
          </Button>
          <div className="set-expiry">
            <Progress value={expiryPct(left)} label="Time left on this code" />
            <span className="t-small text-fg-3 tabular-nums">
              <span aria-hidden="true">{left > 0 ? `Expires in ${formatCountdown(left)}` : 'Expired: connect again for a new code'}</span>
              <span className="sr-only">{left > 0 ? `Expires ${spokenTimeLeft(left)}` : 'Expired: connect again for a new code'}</span>
            </span>
          </div>
        </div>

        <ol className="howto-steps set-device-steps">
          <li>
            <span className="n" aria-hidden="true">
              1
            </span>
            <div className="flex min-w-0 flex-col gap-2">
              <strong>
                Go to <span className="t-mono text-accent-text">{site}</span>
              </strong>
              <span className="t-small text-fg-2">On this phone, your laptop, anywhere you’re signed in to Trakt.</span>
              <a className="btn btn-primary set-open-trakt" href={code.verification_url} target="_blank" rel="noopener noreferrer">
                <ExternalLink size={17} aria-hidden="true" /> Open {site}
              </a>
            </div>
          </li>
          <li>
            <span className="n" aria-hidden="true">
              2
            </span>
            <div className="flex min-w-0 flex-col gap-1">
              <strong>Enter the code and approve</strong>
              <span className="t-small text-fg-2">Trakt will ask to let momoshtrem read your history. Say yes.</span>
            </div>
          </li>
          <li>
            <span className="n" aria-hidden="true">
              3
            </span>
            <div className="set-noir flex min-w-0 gap-3">
              <span className="eyes-box" aria-hidden="true">
                <CatEyes width={34} />
              </span>
              <span className="t-small text-fg-2">I’ll notice as soon as you approve. Keep this page open.</span>
            </div>
          </li>
        </ol>
      </div>
      <div className="set-card-foot">
        <Button variant="ghost" loading={cancelling} onClick={onCancel}>
          Cancel
        </Button>
        <Button onClick={onApproved}>
          <Check size={17} aria-hidden="true" /> I’ve approved it
        </Button>
      </div>
    </Panel>
  );
}
