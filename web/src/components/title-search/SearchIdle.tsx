'use client';

import Link from 'next/link';
import { History, X } from 'lucide-react';
import { Button, IconButton } from '@/components/ui';
import { DEFAULT_FILTERS } from '@/lib/discover/filters';
import { applyMood, MOODS } from '@/lib/discover/moods';
import { discoverHref } from '@/lib/discover/url';
import { addRecentSearch, clearRecentSearches, removeRecentSearch } from '@/lib/title-search/recent';
import { searchHref } from '@/lib/title-search/url';
import { TrendingSection } from './TrendingSection';
import { useRecentSearches } from './useRecentSearches';

/**
 * Search before typing (search.js `idleBody`): recent searches, the moods
 * (Discover's filter presets) and this week's trending titles. The "Ask
 * momoshtrem" chip joins with #9.
 */
export function SearchIdle() {
  const recent = useRecentSearches();
  return (
    <div className="srch-idle">
      <section className="section" aria-labelledby="srch-recent-h">
        <div className="section-head">
          <div className="titles">
            <h2 className="t-title-3" id="srch-recent-h">
              Recent searches
            </h2>
          </div>
          {recent.length > 0 && (
            <div className="actions">
              <Button variant="ghost" size="sm" onClick={clearRecentSearches}>
                Clear
              </Button>
            </div>
          )}
        </div>
        {recent.length > 0 ? (
          <ul className="srch-recent">
            {recent.map((entry) => (
              <li key={entry}>
                <Link href={searchHref(entry)} onClick={() => addRecentSearch(entry)}>
                  <History size={18} aria-hidden="true" />
                  <span>{entry}</span>
                </Link>
                <IconButton size="sm" aria-label={`Remove ${entry} from recent searches`} onClick={() => removeRecentSearch(entry)}>
                  <X size={16} aria-hidden="true" />
                </IconButton>
              </li>
            ))}
          </ul>
        ) : (
          <p className="t-small text-fg-3">No recent searches.</p>
        )}
      </section>

      <section className="section" aria-labelledby="srch-mood-h">
        <div className="section-head">
          <div className="titles">
            <h2 className="t-title-3" id="srch-mood-h">
              Not sure what you want?
            </h2>
            <p className="t-small text-fg-3">Browse Discover by mood.</p>
          </div>
        </div>
        <div className="fchips srch-moods">
          {MOODS.map((mood) => (
            <Link key={mood.id} className="fchip" href={discoverHref(applyMood(DEFAULT_FILTERS, mood.id))}>
              {mood.label}
            </Link>
          ))}
        </div>
      </section>

      <TrendingSection className="srch-full" title="Trending this week" subtitle="From TMDB" count={12} headingId="srch-trend-h" />
    </div>
  );
}
