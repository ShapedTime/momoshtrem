'use client';

import { type FormEvent, type MouseEvent, useRef } from 'react';
import { debounce, useQueryStates } from 'nuqs';
import { Search, X } from 'lucide-react';
import { IconButton, Input, Kbd } from '@/components/ui';
import { DEBOUNCE_DELAY } from '@/config/ui';
import { useDebounce } from '@/hooks/useDebounce';
import { useDocumentTitle } from '@/hooks/useDocumentTitle';
import { addRecentSearch } from '@/lib/title-search/recent';
import type { SearchTab } from '@/lib/title-search/results';
import { cleanQuery } from '@/lib/title-search/text';
import { searchParsers } from '@/lib/title-search/url';
import { plural } from '@/lib/title/format';
import { SearchIdle } from './SearchIdle';
import { SearchNone } from './SearchNone';
import { SearchSkeleton, TitleSearchResults } from './TitleSearchResults';
import { useTitleSearch } from './useTitleSearch';

/**
 * /search (PLAN F6, #101; screens/search, js/screens/search.js): shows,
 * movies and people on TMDB, with your library's matches first. The query
 * and the tab live in the URL (`?q=&tab=`, written once typing pauses).
 * Before typing: recent searches, moods and trending; a search that finds
 * nothing offers tips and what's popular.
 */
export function TitleSearchView() {
  const [{ q, tab }, setParams] = useQueryStates(searchParsers, { history: 'replace', scroll: false });
  const input = useRef<HTMLInputElement>(null);
  const query = cleanQuery(useDebounce(q, DEBOUNCE_DELAY)) ?? '';
  const typing = q.trim() !== '';
  const search = useTitleSearch(typing ? query : '');
  const shown = search.shown;
  useDocumentTitle(query ? `“${query}”` : 'Search');

  // Typing writes the URL once it pauses; clearing and tabs write it at once
  const setQuery = (value: string) =>
    void setParams({ q: value || null, tab: value.trim() ? tab : null }, value ? { limitUrlUpdates: debounce(DEBOUNCE_DELAY) } : undefined);
  const setTab = (next: SearchTab) => void setParams({ tab: next === 'all' ? null : next });
  const clear = () => {
    setQuery('');
    input.current?.focus();
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    const submitted = cleanQuery(q);
    if (submitted) addRecentSearch(submitted);
    input.current?.blur();
  };
  // Opening a result (or acting on one) keeps the search in Recent
  const remember = (event: MouseEvent) => {
    if (!shown || shown.sections.counts.all === 0 || !(event.target instanceof Element)) return;
    if (event.target.closest('a, button:not([role="tab"])')) addRecentSearch(shown.query);
  };

  return (
    <div className="srch page">
      <header className="srch-head">
        <h1>Search</h1>
        <form className="srch-form" role="search" onSubmit={submit}>
          <Input
            ref={input}
            type="search"
            size="lg"
            value={q}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Shows, movies, people"
            aria-label="Search shows, movies and people"
            autoFocus={!typing}
            autoComplete="off"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            enterKeyHint="search"
            leadingIcon={<Search size={20} aria-hidden="true" />}
            end={
              q ? (
                <IconButton aria-label="Clear search" onClick={clear}>
                  <X size={18} aria-hidden="true" />
                </IconButton>
              ) : undefined
            }
          />
        </form>
        <p className="srch-tip t-small text-fg-3">
          Tip: press <Kbd>⌘</Kbd>
          <Kbd>K</Kbd> anywhere to search and jump.
        </p>
      </header>

      <div className="srch-body" onClickCapture={remember}>
        {!typing ? (
          <SearchIdle />
        ) : !shown ? (
          <SearchSkeleton />
        ) : shown.sections.counts.all === 0 && !shown.tmdbError ? (
          <SearchNone query={shown.query} />
        ) : (
          <TitleSearchResults shown={shown} tab={tab} onTab={setTab} onRetry={search.retry} retrying={search.retrying} />
        )}
        {shown && (
          <p className="sr-only" role="status">
            {plural(shown.sections.counts.all, 'result')} for “{shown.query}”
          </p>
        )}
      </div>
    </div>
  );
}
