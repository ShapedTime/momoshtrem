'use client';

import { useState, useEffect, useCallback, FormEvent } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Link from 'next/link';
import { MenuIcon, XIcon } from '@/components/ui';

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

interface NavLinkProps {
  href: string;
  active: boolean;
  children: React.ReactNode;
  onClick?: () => void;
  mobile?: boolean;
}

function NavLink({ href, active, children, onClick, mobile = false }: NavLinkProps) {
  if (mobile) {
    return (
      <Link
        href={href}
        onClick={onClick}
        aria-current={active ? 'page' : undefined}
        className={`
          block w-full px-4 py-3 text-base font-medium
          transition-colors
          focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue focus-visible:ring-inset
          ${active
            ? 'text-white bg-bg-hover'
            : 'text-text-secondary hover:text-white hover:bg-bg-hover'
          }
        `}
      >
        {children}
      </Link>
    );
  }

  return (
    <Link
      href={href}
      aria-current={active ? 'page' : undefined}
      className={`
        text-sm font-medium transition-colors
        focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
        rounded-sm px-1 pb-1
        ${active
          ? 'text-white border-b-2 border-accent-blue'
          : 'text-text-secondary hover:text-white'
        }
      `}
    >
      {children}
    </Link>
  );
}

export function Header() {
  const router = useRouter();
  const pathname = usePathname();
  const [searchQuery, setSearchQuery] = useState('');
  const [isSearchExpanded, setIsSearchExpanded] = useState(false);
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);

  // Close mobile menu on route change
  useEffect(() => {
    setIsMobileMenuOpen(false);
  }, [pathname]);

  // Handle Escape key to close mobile menu
  useEffect(() => {
    function handleEscape(e: KeyboardEvent) {
      if (e.key === 'Escape' && isMobileMenuOpen) {
        setIsMobileMenuOpen(false);
      }
    }

    if (isMobileMenuOpen) {
      document.addEventListener('keydown', handleEscape);
      // Prevent body scroll when menu is open
      document.body.style.overflow = 'hidden';
    }

    return () => {
      document.removeEventListener('keydown', handleEscape);
      document.body.style.overflow = '';
    };
  }, [isMobileMenuOpen]);

  const closeMobileMenu = useCallback(() => {
    setIsMobileMenuOpen(false);
  }, []);

  function handleSearch(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const trimmed = searchQuery.trim();
    if (trimmed) {
      router.push(`/search?q=${encodeURIComponent(trimmed)}`);
      setSearchQuery('');
      setIsSearchExpanded(false);
    }
  }

  // Determine active route
  const isHome = pathname === '/';
  const isMovies = pathname === '/movies' || pathname.startsWith('/movie/');
  const isTVShows = pathname === '/tv-shows' || pathname.startsWith('/tv/');
  const isLibrary = pathname === '/library';

  return (
    <>
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
              rounded-md flex-shrink-0
            "
          >
            PrettyTVCatalog
          </Link>

          {/* Desktop Navigation */}
          <nav
            aria-label="Main navigation"
            className="hidden sm:flex items-center gap-6"
          >
            <NavLink href="/" active={isHome}>
              Home
            </NavLink>
            <NavLink href="/movies" active={isMovies}>
              Movies
            </NavLink>
            <NavLink href="/tv-shows" active={isTVShows}>
              TV Shows
            </NavLink>
            <NavLink href="/library" active={isLibrary}>
              Library
            </NavLink>
          </nav>

          {/* Right section: Search + Mobile Menu Toggle */}
          <div className="flex items-center gap-2">
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

            {/* Mobile Menu Toggle */}
            <button
              type="button"
              onClick={() => setIsMobileMenuOpen(true)}
              className="
                sm:hidden p-2 -m-2 ml-2
                text-text-secondary hover:text-white
                transition-colors
                focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
                rounded-md
                min-w-[44px] min-h-[44px]
                flex items-center justify-center
              "
              aria-label="Open menu"
              aria-expanded={isMobileMenuOpen}
              aria-controls="mobile-menu"
            >
              <MenuIcon size={24} />
            </button>
          </div>
        </div>
      </header>

      {/* Mobile Menu Overlay */}
      {isMobileMenuOpen && (
        <div
          className="fixed inset-0 z-50 sm:hidden"
          role="dialog"
          aria-modal="true"
          aria-label="Navigation menu"
          id="mobile-menu"
        >
          {/* Backdrop */}
          <div
            className="
              absolute inset-0 bg-black/70
              motion-safe:animate-in motion-safe:fade-in motion-safe:duration-200
            "
            onClick={closeMobileMenu}
            aria-hidden="true"
          />

          {/* Drawer */}
          <nav
            aria-label="Mobile navigation"
            className="
              absolute top-0 right-0 bottom-0 w-64
              bg-bg-primary border-l border-border
              motion-safe:animate-in motion-safe:slide-in-from-right motion-safe:duration-200
            "
          >
            {/* Close button */}
            <div className="flex items-center justify-between px-4 h-16 border-b border-border">
              <span className="text-white font-semibold">Menu</span>
              <button
                type="button"
                onClick={closeMobileMenu}
                className="
                  p-2 -m-2
                  text-text-secondary hover:text-white
                  transition-colors
                  focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
                  rounded-md
                  min-w-[44px] min-h-[44px]
                  flex items-center justify-center
                "
                aria-label="Close menu"
              >
                <XIcon size={24} />
              </button>
            </div>

            {/* Nav links */}
            <div className="py-2">
              <NavLink href="/" active={isHome} onClick={closeMobileMenu} mobile>
                Home
              </NavLink>
              <NavLink href="/movies" active={isMovies} onClick={closeMobileMenu} mobile>
                Movies
              </NavLink>
              <NavLink href="/tv-shows" active={isTVShows} onClick={closeMobileMenu} mobile>
                TV Shows
              </NavLink>
              <NavLink href="/library" active={isLibrary} onClick={closeMobileMenu} mobile>
                Library
              </NavLink>
            </div>
          </nav>
        </div>
      )}
    </>
  );
}
