'use client';

import { useState, useEffect, useCallback, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useSearch } from '@/hooks/useTMDB';
import { useDebounce } from '@/hooks/useDebounce';
import { SearchBar, SearchResults, SearchResultsSkeleton } from '@/components/search';
import { SearchIcon } from '@/components/ui';
import { DEBOUNCE_DELAY } from '@/config/ui';

function SearchPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();

  // Get initial query from URL
  const initialQuery = searchParams.get('q') || '';
  const [inputValue, setInputValue] = useState(initialQuery);

  // Debounce the input value
  const debouncedQuery = useDebounce(inputValue, DEBOUNCE_DELAY);

  // Use the search hook
  const { data: results, isLoading, error, search, clearResults } = useSearch();

  // Update URL when debounced query changes
  useEffect(() => {
    const currentQuery = searchParams.get('q') || '';
    if (debouncedQuery !== currentQuery) {
      if (debouncedQuery) {
        router.replace(`/search?q=${encodeURIComponent(debouncedQuery)}`, { scroll: false });
      } else {
        router.replace('/search', { scroll: false });
      }
    }
  }, [debouncedQuery, router, searchParams]);

  // Trigger search when debounced query changes
  useEffect(() => {
    if (debouncedQuery) {
      search(debouncedQuery);
    } else {
      clearResults();
    }
  }, [debouncedQuery, search, clearResults]);

  // Handle clear
  const handleClear = useCallback(() => {
    setInputValue('');
    clearResults();
    router.replace('/search', { scroll: false });
  }, [clearResults, router]);

  // Determine what to show
  const showEmptyQueryMessage = !inputValue.trim() && !isLoading;
  const showLoading = isLoading && !!inputValue;
  const showResults = results && results.length > 0 && !isLoading;
  const showNoResults = results && results.length === 0 && debouncedQuery && !isLoading;
  const showError = error && !isLoading;

  return (
    <div className="px-4 sm:px-6 lg:px-12 py-6 sm:py-8">
      {/* Search input */}
      <div className="mb-8 sm:mb-12">
        <SearchBar
          value={inputValue}
          onChange={setInputValue}
          onClear={handleClear}
          isLoading={showLoading}
          autoFocus
        />
      </div>

      {/* Empty query state */}
      {showEmptyQueryMessage && (
        <div className="flex flex-col items-center justify-center py-16">
          <SearchIcon size={64} className="text-text-muted" strokeWidth={1} />
          <p className="text-text-secondary text-lg mt-4">
            Start typing to search for movies and TV shows
          </p>
        </div>
      )}

      {/* Error state */}
      {showError && (
        <div className="flex flex-col items-center justify-center py-16">
          <p className="text-red-400 text-lg">{error}</p>
        </div>
      )}

      {/* Loading state */}
      {showLoading && <SearchResultsSkeleton />}

      {/* Results */}
      {showResults && <SearchResults results={results} />}

      {/* No results */}
      {showNoResults && (
        <SearchResults
          results={[]}
          emptyMessage={`No results found for "${debouncedQuery}"`}
        />
      )}
    </div>
  );
}

// Loading fallback for Suspense
function SearchPageLoading() {
  return (
    <div className="px-4 sm:px-6 lg:px-12 py-6 sm:py-8">
      <div className="w-full max-w-2xl mx-auto mb-8 sm:mb-12">
        <div className="h-14 bg-bg-elevated rounded-lg animate-pulse motion-reduce:animate-none" />
      </div>
      <SearchResultsSkeleton />
    </div>
  );
}

export default function SearchPage() {
  return (
    <Suspense fallback={<SearchPageLoading />}>
      <SearchPageContent />
    </Suspense>
  );
}
