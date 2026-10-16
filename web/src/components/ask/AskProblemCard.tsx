import { CircleAlert, Clock, Coins, Hourglass, KeyRound, type LucideIcon, MessagesSquare, RotateCw, SearchX, Settings, WifiOff } from 'lucide-react';
import { Button, ButtonLink } from '@/components/ui';
import type { AskProblem, AskProblemIcon } from '@/lib/ask/copy';
import { cx } from '@/lib/utils/cx';

const ICONS: Record<AskProblemIcon, LucideIcon> = {
  budget: Coins,
  limit: Clock,
  full: MessagesSquare,
  busy: Hourglass,
  setup: KeyRound,
  gone: SearchX,
  offline: WifiOff,
  error: CircleAlert,
};

interface AskProblemCardProps {
  problem: AskProblem;
  /** Try again: resend the message. Offered only on the conversation's last turn. */
  onRetry?: () => void;
  /** New conversation. */
  onNew?: () => void;
}

/** A turn without its answer (ask.js `errorTurn`): what happened, what still works, and the way forward. */
export function AskProblemCard({ problem, onRetry, onNew }: AskProblemCardProps) {
  const Icon = ICONS[problem.icon];
  const actions = problem.actions.filter((action) => (action === 'retry' ? onRetry : action === 'new' ? onNew : true));
  return (
    // The page's live region announces it as it happens; a thread read later stays quiet
    <div className={cx('ask-error', problem.tone === 'error' && 'is-error')}>
      <span className="ask-error-ic" aria-hidden="true">
        <Icon size={22} />
      </span>
      <div className="flex min-w-0 flex-col gap-2">
        <h3 className="t-title-3">{problem.title}</h3>
        <p className="text-fg-2">{problem.body}</p>
        {actions.length > 0 && (
          <div className="ask-error-actions">
            {actions.map((action) => {
              switch (action) {
                case 'settings':
                  return (
                    <ButtonLink key={action} size="sm" href="/settings?section=ai">
                      <Settings size={15} aria-hidden="true" />
                      Settings › AI
                    </ButtonLink>
                  );
                case 'discover':
                  return (
                    <ButtonLink key={action} variant="ghost" size="sm" href="/discover">
                      Browse Discover instead
                    </ButtonLink>
                  );
                case 'new':
                  return (
                    <Button key={action} size="sm" onClick={onNew}>
                      <MessagesSquare size={15} aria-hidden="true" />
                      New conversation
                    </Button>
                  );
                case 'retry':
                  return (
                    <Button key={action} size="sm" onClick={onRetry}>
                      <RotateCw size={15} aria-hidden="true" />
                      Try again
                    </Button>
                  );
              }
            })}
          </div>
        )}
      </div>
    </div>
  );
}
