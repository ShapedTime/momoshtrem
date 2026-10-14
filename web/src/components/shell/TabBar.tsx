'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { isCurrent, NAV } from '@/lib/nav';

/** Phone and tablet (< 1024): fixed glass tab bar above the home indicator. */
export function TabBar({ activeCount }: { activeCount: number }) {
  const pathname = usePathname();
  return (
    <nav className="tabbar" aria-label="Main">
      {NAV.map((item) => {
        const current = isCurrent(item, pathname);
        const Icon = item.icon;
        return (
          <Link key={item.id} href={item.href} aria-current={current ? 'page' : undefined}>
            <Icon size={24} strokeWidth={current ? 2 : 1.6} aria-hidden="true" />
            <span>{item.label}</span>
            {item.id === 'activity' && activeCount > 0 && (
              <span className="dot" aria-label={`${activeCount} downloading`}>
                {activeCount}
              </span>
            )}
          </Link>
        );
      })}
    </nav>
  );
}
