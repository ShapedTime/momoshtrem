import Link from 'next/link';
import { Compass, Info, User } from 'lucide-react';
import { EmptyState } from '@/components/ui';
import { TrendingSection } from './TrendingSection';

/**
 * A search that found nothing (search.js `noneBody`): what to try instead,
 * then what's popular. "Ask momoshtrem" joins with #9.
 */
export function SearchNone({ query }: { query: string }) {
  return (
    <div className="srch-none">
      <EmptyState cat="sit" title={`Nothing for “${query}”`} headingLevel="h2">
        Check the spelling, or try fewer words.
      </EmptyState>
      <ul className="srch-tips panel panel-pad" aria-label="Search tips">
        <li>
          <Info size={16} aria-hidden="true" />
          <span>Original titles work too: “sousou no frieren” finds Frieren.</span>
        </li>
        <li>
          <User size={16} aria-hidden="true" />
          <span>Search people: “wim wenders”, “vince gilligan”.</span>
        </li>
        <li>
          <Compass size={16} aria-hidden="true" />
          <span>
            Know the vibe, not the name?{' '}
            <Link className="text-accent-text underline underline-offset-2" href="/discover">
              Browse Discover
            </Link>
            .
          </span>
        </li>
      </ul>
      <TrendingSection title="Popular right now" count={6} headingId="srch-pop-h" />
    </div>
  );
}
