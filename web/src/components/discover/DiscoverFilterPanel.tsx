'use client';

import { type ReactNode, useId } from 'react';
import { Star } from 'lucide-react';
import { FilterChip, SegmentedControl, Select, Switch } from '@/components/ui';
import {
  COUNTRIES,
  COUNTRY_LABELS,
  currentYear,
  DECADES,
  decadeOf,
  DISCOVER_SORTS,
  DISCOVER_TYPES,
  type DiscoverFilters,
  LANGUAGE_LABELS,
  LANGUAGES,
  type RatingFloor,
  RATINGS,
  RUNTIME_LABELS,
  type RuntimeBand,
  RUNTIMES,
  type ShowStatus,
  SORT_LABELS,
  STATUS_LABELS,
  STATUSES,
  toggleDecade,
  TYPE_LABELS,
  YEAR_MIN,
} from '@/lib/discover/filters';
import { GENRES } from '@/lib/discover/genres';

interface DiscoverFilterPanelProps {
  filters: DiscoverFilters;
  onChange: (next: DiscoverFilters) => void;
  /** The sheet adds Sort (the desktop sort sits in the results header). */
  variant: 'side' | 'sheet';
}

/** "Short" etc. per band, as the runtime help text explains them. */
const RUNTIME_HELP = 'Short: 30 min or less an episode, or under 100 min for a movie. Long: over 50 min, or over 2 h 20 m.';

/**
 * Every filter, in the design's order (discover.js `panel`): the desktop
 * sidebar applies each change at once; the phone sheet stages them.
 */
export function DiscoverFilterPanel({ filters, onChange, variant }: DiscoverFilterPanelProps) {
  const id = useId();
  const set = (patch: Partial<DiscoverFilters>) => onChange({ ...filters, ...patch });
  const isMovie = filters.type === 'movie';
  const years = yearOptions([filters.from, filters.to]);

  return (
    <div className="disc-panel">
      <Section label="Type">
        <SegmentedControl
          block
          aria-label="Type"
          value={filters.type}
          onChange={(type) => set({ type })}
          options={DISCOVER_TYPES.map((value) => ({ value, label: TYPE_LABELS[value] }))}
        />
      </Section>

      {variant === 'sheet' && (
        <Section label="Sort by">
          <div className="fchips">
            {DISCOVER_SORTS.map((sort) => (
              <FilterChip key={sort} pressed={filters.sort === sort} onPressedChange={() => set({ sort })}>
                {SORT_LABELS[sort]}
              </FilterChip>
            ))}
          </div>
        </Section>
      )}

      <Section label="Genres" hint="match all selected">
        <div className="fchips">
          {GENRES.map((genre) => {
            const on = filters.genres.includes(genre.slug);
            return (
              <FilterChip
                key={genre.slug}
                pressed={on}
                onPressedChange={() => set({ genres: on ? filters.genres.filter((slug) => slug !== genre.slug) : [...filters.genres, genre.slug] })}
              >
                {genre.label}
              </FilterChip>
            );
          })}
        </div>
      </Section>

      <Section label="Release year">
        <div className="disc-years">
          <Select aria-label="From year" value={filters.from ?? ''} onChange={(event) => set({ from: toYear(event.target.value) })}>
            <option value="">From</option>
            {years.map((year) => (
              <option key={year} value={year}>
                {year}
              </option>
            ))}
          </Select>
          <span className="text-fg-3" aria-hidden="true">
            –
          </span>
          <Select aria-label="To year" value={filters.to ?? ''} onChange={(event) => set({ to: toYear(event.target.value) })}>
            <option value="">To</option>
            {years.map((year) => (
              <option key={year} value={year}>
                {year}
              </option>
            ))}
          </Select>
        </div>
        <div className="fchips" role="group" aria-label="Decades">
          {DECADES.map((decade) => (
            <FilterChip key={decade.label} pressed={decadeOf(filters) === decade} onPressedChange={() => onChange(toggleDecade(filters, decade))}>
              {decade.label}
            </FilterChip>
          ))}
        </div>
      </Section>

      <Section label="Rating">
        <SegmentedControl<'' | `${RatingFloor}`>
          block
          aria-label="Minimum rating"
          value={filters.rating === null ? '' : `${filters.rating}`}
          onChange={(value) => set({ rating: value === '' ? null : (Number(value) as RatingFloor) })}
          options={[
            { value: '', label: 'Any' },
            ...RATINGS.map((rating) => ({
              value: `${rating}` as const,
              label: (
                <>
                  <Star size={12} fill="currentColor" aria-hidden="true" />
                  {rating}+
                </>
              ),
            })),
          ]}
        />
      </Section>

      <Section label="Runtime">
        <SegmentedControl<'' | RuntimeBand>
          block
          aria-label="Runtime"
          value={filters.runtime ?? ''}
          onChange={(value) => set({ runtime: value === '' ? null : value })}
          options={[{ value: '', label: 'Any' }, ...RUNTIMES.map((value) => ({ value, label: RUNTIME_LABELS[value] }))]}
        />
        <p className="help">{RUNTIME_HELP}</p>
      </Section>

      <Section label="Language & country">
        <div className="disc-anime">
          <div className="grow">
            <label htmlFor={`${id}-anime`} className="font-semibold text-fg-1">
              Anime
            </label>
            <span className="help" id={`${id}-anime-help`}>
              Japanese animation, shows and films
            </span>
          </div>
          <Switch id={`${id}-anime`} checked={filters.anime} onCheckedChange={(anime) => set({ anime })} describedBy={`${id}-anime-help`} />
        </div>
        <div className="fchips" role="group" aria-label="Original language">
          {LANGUAGES.map((lang) => (
            <FilterChip key={lang} pressed={filters.lang === lang} onPressedChange={() => set({ lang: filters.lang === lang ? null : lang })}>
              {LANGUAGE_LABELS[lang]}
            </FilterChip>
          ))}
        </div>
        <Select
          aria-label="Country of origin"
          value={filters.country ?? ''}
          onChange={(event) => set({ country: COUNTRIES.find((code) => code === event.target.value) ?? null })}
        >
          <option value="">Any country</option>
          {COUNTRIES.map((code) => (
            <option key={code} value={code}>
              {COUNTRY_LABELS[code]}
            </option>
          ))}
        </Select>
      </Section>

      <Section label="Status" disabled={isMovie}>
        <SegmentedControl<'' | ShowStatus>
          block
          aria-label="Show status"
          value={isMovie ? '' : (filters.status ?? '')}
          onChange={(value) => set({ status: value === '' ? null : value })}
          options={[{ value: '', label: 'Any' }, ...STATUSES.map((value) => ({ value, label: STATUS_LABELS[value] }))]}
        />
        {isMovie && <p className="help">For shows only.</p>}
      </Section>
    </div>
  );
}

function Section({ label, hint, disabled, children }: { label: string; hint?: string; disabled?: boolean; children: ReactNode }) {
  return (
    <fieldset className="disc-sec" disabled={disabled}>
      <legend className="t-micro text-fg-3">
        {label}
        {hint && <span className="disc-hint">{hint}</span>}
      </legend>
      {children}
    </fieldset>
  );
}

const toYear = (value: string) => (value === '' ? null : Number(value));

/** This year back to YEAR_MIN, plus any year in the URL outside that range. */
function yearOptions(selected: Array<number | null>): number[] {
  const years: number[] = [];
  for (let year = currentYear(); year >= YEAR_MIN; year--) years.push(year);
  for (const year of selected) if (year !== null && !years.includes(year)) years.push(year);
  return years.sort((a, b) => b - a);
}
