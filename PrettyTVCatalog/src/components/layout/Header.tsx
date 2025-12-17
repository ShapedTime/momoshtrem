'use client';

import { useState, FormEvent } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';

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
    >
      <circle cx="11" cy="11" r="8" />
      <line x1="21" y1="21" x2="16.65" y2="16.65" />
    </svg>
  );
}

export function Header() {
  const router = useRouter();
  const [searchQuery, setSearchQuery] = useState('');
  const [isSearchExpanded, setIsSearchExpanded] = useState(false);

  function handleSearch(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const trimmed = searchQuery.trim();
    if (trimmed) {
      router.push(`/search?q=${encodeURIComponent(trimmed)}`);
      setSearchQuery('');
      setIsSearchExpanded(false);
    }
  }

  return (
    <header
      className="
        sticky top-0 z-40
        bg-gradient-to-b from-bg-primary via-bg-primary/95 to-transparent
        backdrop-blur-sm
      "
    >
      <div
        className="
          flex items-center justify-between gap-4
          px-4 sm:px-6 lg:px-12
          h-16 sm:h-20
          max-w-screen-2xl mx-auto
        "
      >
        {/* Logo */}
        <Link
          href="/"
          className="
            text-xl sm:text-2xl font-bold text-white
            hover:text-text-secondary transition-colors
            focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
            rounded-md
          "
        >
          PrettyTVCatalog
        </Link>

        {/* Search */}
        <form
          onSubmit={handleSearch}
          className={`
            flex items-center
            ${isSearchExpanded ? 'flex-1 max-w-md' : ''}
          `}
        >
          {/* Mobile: Icon button that expands */}
          <button
            type="button"
            onClick={() => setIsSearchExpanded(!isSearchExpanded)}
            className="
              sm:hidden p-2 -m-2
              text-text-secondary hover:text-white
              transition-colors
              focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
              rounded-md
            "
            aria-label="Toggle search"
          >
            <SearchIcon />
          </button>

          {/* Search input */}
          <div
            className={`
              ${isSearchExpanded ? 'flex' : 'hidden'} sm:flex
              items-center gap-2
              flex-1 sm:flex-none
              bg-bg-elevated border border-border rounded-md
              px-3 h-10
              focus-within:ring-2 focus-within:ring-accent-blue focus-within:border-transparent
              transition-all
            `}
          >
            <SearchIcon />
            <input
              type="search"
              placeholder="Search movies & TV shows..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="
                flex-1 bg-transparent border-none outline-none
                text-white placeholder-text-muted
                text-sm
                min-w-0 w-full sm:w-48 lg:w-64
              "
            />
          </div>
        </form>
      </div>
    </header>
  );
}
