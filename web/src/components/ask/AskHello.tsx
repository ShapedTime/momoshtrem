import Link from 'next/link';
import { MessageCircle, Sparkle } from 'lucide-react';
import { CatAvatar } from '@/components/brand';
import { ButtonLink, EmptyState } from '@/components/ui';
import { ASK_EXAMPLES } from '@/lib/ask/copy';
import { askHref } from '@/lib/recommendations/ask-href';
import { cx } from '@/lib/utils/cx';

/** A new conversation (ask.js `hello`): Noir, what to ask, and six examples that start one. */
export function AskHello() {
  return (
    <div className="ask-hello">
      <div className="ask-hello-cat">
        <CatAvatar size={88} />
      </div>
      <h1 className="t-display">What are we in the mood for?</h1>
      <p className="t-body-l text-fg-2">Ask in your own words. I read your Trakt history, check what’s new, and tell you why each pick fits.</p>
      <ul className="ask-examples" aria-label="Example asks">
        {ASK_EXAMPLES.map((example, index) => {
          const Icon = index < 3 ? Sparkle : MessageCircle;
          return (
            <li key={example}>
              <Link className={cx('ask-example', index < 3 && 'is-main')} href={askHref(example)}>
                <Icon size={15} aria-hidden="true" />
                <span>{example}</span>
              </Link>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

/** Without ANTHROPIC_API_KEY (`/recommendations/home` `configured: false`): what Ask needs. Past conversations still read. */
export function AskNotConfigured() {
  return (
    <EmptyState
      cat="sleep"
      title="Ask momoshtrem needs an Anthropic API key"
      headingLevel="h1"
      actions={
        <ButtonLink href="/settings?section=ai" variant="secondary">
          Settings › AI
        </ButtonLink>
      }
    >
      Add ANTHROPIC_API_KEY to the server’s .env and restart it. Your library, torrents and Infuse work without it.
    </EmptyState>
  );
}
