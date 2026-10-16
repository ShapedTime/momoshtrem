'use client';

import Link from 'next/link';
import { Compass, Info, User } from 'lucide-react';
import { CatAvatar } from '@/components/brand';
import { ButtonLink, EmptyState } from '@/components/ui';
import { useAskAvailable } from '@/hooks/useAskAvailable';
import { askHref } from '@/lib/recommendations/ask-href';
import { TrendingSection } from './TrendingSection';

/**
 * A search that found nothing (search.js `noneBody`): what to try instead,
 * asking Noir about it (with an Anthropic key), then what's popular.
 */
export function SearchNone({ query }: { query: string }) {
  const askAvailable = useAskAvailable();
  return (
    <div className="srch-none">
      <EmptyState
        cat="sit"
        title={`Nothing for “${query}”`}
        headingLevel="h2"
        actions={
          askAvailable ? (
            <ButtonLink href={askHref(query)}>
              <CatAvatar size={22} />
              Ask momoshtrem
            </ButtonLink>
          ) : undefined
        }
      >
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
