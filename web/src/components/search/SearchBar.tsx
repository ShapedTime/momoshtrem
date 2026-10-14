'use client';

import { useRef, useEffect } from 'react';

interface SearchBarProps {
  value: string;
  onChange: (value: string) => void;
  onClear: () => void;
  isLoading?: boolean;
  autoFocus?: boolean;
  placeholder?: string;
}

function SearchIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className="text-text-muted"
    >
      <circle cx="11" cy="11" r="8" />
      <line x1="21" y1="21" x2="16.65" y2="16.65" />
    </svg>
  );
}

function ClearIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <line x1="18" y1="6" x2="6" y2="18" />
      <line x1="6" y1="6" x2="18" y2="18" />
    </svg>
  );
}

function LoadingSpinner() {
  return (
    <div
      className="w-5 h-5 border-2 border-text-muted border-t-white rounded-full animate-spin"
      role="status"
      aria-label="Searching"
    />
  );
}

export function SearchBar({
  value,
  onChange,
  onClear,
  isLoading = false,
  autoFocus = false,
  placeholder = 'Search movies & TV shows...',
}: SearchBarProps) {
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (autoFocus && inputRef.current) {
      inputRef.current.focus();
    }
  }, [autoFocus]);

  return (
    <div className="w-full max-w-2xl mx-auto">
      <div
        className="
          flex items-center gap-3
          bg-bg-elevated border border-border rounded-lg
          px-4 h-14
          focus-within:ring-2 focus-within:ring-accent-blue focus-within:border-transparent
          transition-all
        "
      >
        <SearchIcon />
        <input
          ref={inputRef}
          type="search"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          className="
            flex-1 bg-transparent border-none outline-none
            text-white placeholder-text-muted
            text-base
          "
          aria-label="Search"
        />
        {value && !isLoading && (
          <button
            type="button"
            onClick={onClear}
            className="
              p-1.5 text-text-muted hover:text-white
              transition-colors rounded
              focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
            "
            aria-label="Clear search"
          >
            <ClearIcon />
          </button>
        )}
        {isLoading && <LoadingSpinner />}
      </div>
    </div>
  );
}
