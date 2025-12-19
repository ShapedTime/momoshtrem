'use client';

import { useState, useEffect } from 'react';
import { FilterIcon, ChevronLeftIcon, XIcon } from '@/components/ui';
import type { Genre, MediaType, SortOption } from '@/types/tmdb';
import { GenreFilter } from './GenreFilter';
import { SortSelect } from './SortSelect';

interface FilterSidebarProps {
  mediaType: MediaType;
  genres: Genre[];
  genresLoading?: boolean;
  selectedGenre: number | null;
  onSelectGenre: (genreId: number | null) => void;
  sortBy: SortOption;
  onSortChange: (sortBy: SortOption) => void;
}

export function FilterSidebar({
  mediaType,
  genres,
  genresLoading = false,
  selectedGenre,
  onSelectGenre,
  sortBy,
  onSortChange,
}: FilterSidebarProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [isMobile, setIsMobile] = useState(false);

  // Detect mobile viewport
  useEffect(() => {
    const checkMobile = () => {
      setIsMobile(window.innerWidth < 1024);
    };
    checkMobile();
    window.addEventListener('resize', checkMobile);
    return () => window.removeEventListener('resize', checkMobile);
  }, []);

  // Auto-open sidebar on desktop
  useEffect(() => {
    if (!isMobile) {
      setIsOpen(true);
    }
  }, [isMobile]);

  // Close sidebar when clicking outside on mobile
  useEffect(() => {
    if (isMobile && isOpen) {
      const handleClickOutside = (e: MouseEvent) => {
        const sidebar = document.getElementById('filter-sidebar');
        if (sidebar && !sidebar.contains(e.target as Node)) {
          setIsOpen(false);
        }
      };
      document.addEventListener('mousedown', handleClickOutside);
      return () => document.removeEventListener('mousedown', handleClickOutside);
    }
  }, [isMobile, isOpen]);

  // Prevent body scroll when sidebar is open on mobile
  useEffect(() => {
    if (isMobile && isOpen) {
      document.body.style.overflow = 'hidden';
      return () => {
        document.body.style.overflow = '';
      };
    }
  }, [isMobile, isOpen]);

  const selectedGenreName = selectedGenre
    ? genres.find((g) => g.id === selectedGenre)?.name
    : null;

  const hasActiveFilters = selectedGenre !== null || sortBy !== 'popularity.desc';

  return (
    <>
      {/* Mobile toggle button */}
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="
          lg:hidden fixed bottom-6 right-6 z-40
          flex items-center gap-2 px-4 py-3 rounded-full
          bg-accent-blue text-white shadow-lg
          hover:bg-accent-blue/90 transition-colors
        "
        aria-label="Toggle filters"
      >
        <FilterIcon size={20} />
        <span className="font-medium">Filters</span>
        {hasActiveFilters && (
          <span className="ml-1 w-2 h-2 bg-white rounded-full" />
        )}
      </button>

      {/* Backdrop for mobile */}
      {isMobile && isOpen && (
        <div
          className="fixed inset-0 bg-black/60 z-40 lg:hidden"
          onClick={() => setIsOpen(false)}
        />
      )}

      {/* Sidebar */}
      <aside
        id="filter-sidebar"
        className={`
          fixed lg:sticky top-0 lg:top-20 left-0 z-50 lg:z-0
          h-full lg:h-auto max-h-screen lg:max-h-[calc(100vh-5rem)]
          w-72 lg:w-64
          bg-bg-primary lg:bg-transparent
          transform transition-transform duration-300 ease-in-out
          ${isOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}
          ${!isOpen && !isMobile ? 'lg:hidden' : ''}
          overflow-y-auto
          lg:mr-6
        `}
      >
        <div className="p-6 lg:p-0">
          {/* Header */}
          <div className="flex items-center justify-between mb-6">
            <h2 className="text-lg font-semibold text-white flex items-center gap-2">
              <FilterIcon size={20} />
              Filters
            </h2>
            <button
              onClick={() => setIsOpen(false)}
              className="lg:hidden p-2 text-text-muted hover:text-white transition-colors"
              aria-label="Close filters"
            >
              <XIcon size={20} />
            </button>
          </div>

          {/* Active filter indicator */}
          {selectedGenreName && (
            <div className="mb-6 p-3 bg-bg-elevated rounded-lg">
              <div className="flex items-center justify-between">
                <span className="text-sm text-text-secondary">Active filter:</span>
                <button
                  onClick={() => onSelectGenre(null)}
                  className="text-accent-blue text-sm hover:underline"
                >
                  Clear
                </button>
              </div>
              <span className="text-white text-sm font-medium">
                {selectedGenreName}
              </span>
            </div>
          )}

          {/* Sort */}
          <div className="mb-6">
            <SortSelect
              mediaType={mediaType}
              value={sortBy}
              onChange={onSortChange}
            />
          </div>

          {/* Genres */}
          <GenreFilter
            genres={genres}
            selectedGenre={selectedGenre}
            onSelectGenre={onSelectGenre}
            isLoading={genresLoading}
          />
        </div>
      </aside>

      {/* Desktop sidebar toggle (when collapsed) */}
      {!isMobile && !isOpen && (
        <button
          onClick={() => setIsOpen(true)}
          className="
            hidden lg:flex fixed left-0 top-1/2 -translate-y-1/2 z-30
            items-center justify-center w-10 h-20
            bg-bg-elevated border border-white/10 rounded-r-lg
            text-text-muted hover:text-white transition-colors
          "
          aria-label="Open filters"
        >
          <ChevronLeftIcon size={20} className="rotate-180" />
        </button>
      )}
    </>
  );
}
