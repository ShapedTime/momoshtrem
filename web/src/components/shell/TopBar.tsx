'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useEffect, useState } from 'react';
import { Search } from 'lucide-react';
import { Mark } from '@/components/brand';
import { Kbd } from '@/components/ui';
import { isCurrent, NAV } from '@/lib/nav';
import { cx } from '@/lib/utils/cx';
import { useShell } from './ShellContext';
import { UserMenu } from './UserMenu';

/** Scrolled past `offset` px (passive listener; HANDOFF §4.2: 120). */
function useScrolledPast(offset: number, enabled: boolean): boolean {
  const [past, setPast] = useState(false);
  useEffect(() => {
    if (!enabled) return;
    const update = () => setPast(window.scrollY > offset);
    update();
    window.addEventListener('scroll', update, { passive: true });
    return () => window.removeEventListener('scroll', update);
  }, [offset, enabled]);
  return past;
}

/**
 * Desktop (≥ 1024) glass top bar: brand, nav, search / ⌘K, account. Over a
 * hero it starts transparent and turns solid after 120 px of scroll.
 */
export function TopBar({ username, overArt, activeCount }: { username: string; overArt: boolean; activeCount: number }) {
  const pathname = usePathname();
  const { openPalette } = useShell();
  const scrolled = useScrolledPast(120, overArt);
  const solid = !overArt || scrolled;

  return (
    <header className={cx('topbar', solid ? 'is-solid' : 'is-over-art')}>
      <Link href="/" className="brand rounded-md" aria-label="momoshtrem, For You">
        <Mark size={30} />
        <span>momoshtrem</span>
      </Link>
      <nav aria-label="Main">
        {NAV.filter((item) => item.inTopBar).map((item) => (
          <Link key={item.id} href={item.href} aria-current={isCurrent(item, pathname) ? 'page' : undefined}>
            {item.label}
            {item.id === 'activity' && activeCount > 0 && (
              <span className="badge" aria-label={`${activeCount} downloading`}>
                {activeCount}
              </span>
            )}
          </Link>
        ))}
      </nav>
      <button type="button" className="search-trigger" aria-label="Search titles, your library and actions" onClick={() => openPalette()}>
        <Search size={16} aria-hidden="true" />
        <span>Search or jump to…</span>
        <Kbd>⌘K</Kbd>
      </button>
      {/* "Ask momoshtrem" joins here with #9 */}
      <UserMenu username={username} />
    </header>
  );
}
