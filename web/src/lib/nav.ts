import { Activity, Compass, LibraryBig, type LucideIcon, Search, Sparkles } from 'lucide-react';

export interface NavItem {
  id: 'home' | 'discover' | 'search' | 'library' | 'activity';
  label: string;
  href: string;
  icon: LucideIcon;
  /** Path prefixes that count as "here" (aria-current). */
  match: string[];
  /** Shown in the desktop top bar (Search has its own field there). */
  inTopBar: boolean;
}

/**
 * The five destinations (HANDOFF §2/§3). Until their v2 screens land, the
 * hrefs point at the existing pages: Discover → /movies (#7), Activity →
 * /torrents (#7). Feature issues update their entry here.
 */
export const NAV: NavItem[] = [
  { id: 'home', label: 'For You', href: '/', icon: Sparkles, match: ['/'], inTopBar: true },
  { id: 'discover', label: 'Discover', href: '/movies', icon: Compass, match: ['/movies', '/tv-shows', '/discover'], inTopBar: true },
  { id: 'search', label: 'Search', href: '/search', icon: Search, match: ['/search'], inTopBar: false },
  { id: 'library', label: 'Library', href: '/library', icon: LibraryBig, match: ['/library'], inTopBar: true },
  { id: 'activity', label: 'Activity', href: '/torrents', icon: Activity, match: ['/torrents', '/activity'], inTopBar: true },
];

/** Whether `pathname` belongs to a nav item ("/" only matches exactly). */
export function isCurrent(item: NavItem, pathname: string): boolean {
  return item.match.some((prefix) => (prefix === '/' ? pathname === '/' : pathname === prefix || pathname.startsWith(`${prefix}/`)));
}
