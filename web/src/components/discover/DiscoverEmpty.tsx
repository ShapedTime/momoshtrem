'use client';

import { useQuery } from '@tanstack/react-query';
import { ArrowRight, X } from 'lucide-react';
import { CatAvatar } from '@/components/brand';
import { Button, ButtonLink, EmptyState } from '@/components/ui';
import { useAskAvailable } from '@/hooks/useAskAvailable';
import { discoverRelaxQuery } from '@/lib/api/endpoints';
import { discoverAskQuery } from '@/lib/discover/ask';
import { activeFilters, clearFilters, countLabel, type DiscoverFilters, noun, withoutFilter } from '@/lib/discover/filters';
import { askHref } from '@/lib/recommendations/ask-href';
import { cx } from '@/lib/utils/cx';

/**
 * Filtered to nothing (discover.js, state `empty`): one chip per active
 * filter that removes it, labelled with what you'd get back (`relax=1`) and
 * sorted by it, plus Clear all filters and, with an Anthropic key, Ask
 * momoshtrem for the filters in words ("an anime comedy show that has
 * finished airing").
 */
export function DiscoverEmpty({ filters, onChange }: { filters: DiscoverFilters; onChange: (next: DiscoverFilters) => void }) {
  const active = activeFilters(filters);
  const askAvailable = useAskAvailable();
  const relax = useQuery({ ...discoverRelaxQuery(filters), enabled: active.length > 0 });
  const wouldBe = new Map(relax.data?.relaxed.map((entry) => [entry.filter, entry.would_be]));
  // Unknown counts (still loading, or the call failed) keep the chips in their usual order
  const fixes = active.map((filter) => ({ ...filter, count: wouldBe.get(filter.key) })).sort((a, b) => (b.count ?? -1) - (a.count ?? -1));
  const many = noun(filters.type, 2);
  const title =
    active.length === 0 ? `No ${many} here` : active.length === 1 ? `No ${many} match this filter` : `No ${many} match all ${active.length} filters`;

  return (
    <EmptyState
      title={title}
      className="disc-empty"
      actions={
        <>
          {fixes.length > 0 && (
            <div className="disc-fixes" role="group" aria-label="Remove a filter" aria-busy={relax.isFetching || undefined}>
              {fixes.map(({ key, label, count }) => (
                <button
                  key={key}
                  type="button"
                  className={cx('fchip disc-fix', count !== undefined && count > 0 && 'has-results')}
                  aria-label={count === undefined ? `Remove ${label}` : `Remove ${label}: ${count > 0 ? countLabel(filters.type, count) : `still no ${many}`}`}
                  onClick={() => onChange(withoutFilter(filters, key))}
                >
                  <X size={13} strokeWidth={2.2} aria-hidden="true" />
                  <span>{label}</span>
                  {count !== undefined && (
                    <span className="n">
                      {count > 0 ? (
                        <>
                          <ArrowRight size={12} aria-hidden="true" />
                          {countLabel(filters.type, count)}
                        </>
                      ) : (
                        'still none'
                      )}
                    </span>
                  )}
                </button>
              ))}
            </div>
          )}
          {active.length > 0 && <Button onClick={() => onChange(clearFilters(filters))}>Clear all filters</Button>}
          {askAvailable && (
            <ButtonLink variant="ghost" href={askHref(discoverAskQuery(filters))}>
              <CatAvatar size={22} />
              Ask momoshtrem
            </ButtonLink>
          )}
        </>
      }
    >
      {active.length > 0 ? 'Remove one to widen the net. Each shows what you’d get back.' : 'TMDB has nothing for this view yet.'}
    </EmptyState>
  );
}
