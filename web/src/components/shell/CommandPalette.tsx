'use client';

import { useRouter } from 'next/navigation';
import { type KeyboardEvent, type ReactNode, useState } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import { Command } from 'cmdk';
import { Activity, Compass, History, LibraryBig, Link2, ListChecks, LogOut, RefreshCw, Search, Settings, Tv, type LucideIcon } from 'lucide-react';
import { MarkMono } from '@/components/brand';
import { Highlight } from '@/components/title-search/Highlight';
import { PaletteIcon, PaletteRow, PaletteThumb } from '@/components/title-search/PaletteRow';
import { usePaletteData } from '@/components/title-search/usePaletteData';
import { Kbd, StatusPill, useToast } from '@/components/ui';
import { useRestoreFocus } from '@/hooks/useRestoreFocus';
import { useTraktSync } from '@/hooks/useTraktActions';
import type { LibraryItem, TitleSearchItem } from '@/lib/api/contract';
import type { FindTarget } from '@/lib/library/actions';
import { kindLine } from '@/lib/library/display';
import { SYNC_STARTED_TOAST } from '@/lib/settings/trakt';
import { getSheet } from '@/lib/sheets/registry';
import { useSheet } from '@/lib/sheets/useSheet';
import { findTorrentHref, matchingActions, type PaletteAction, type PaletteActionIcon, type PaletteActionTarget } from '@/lib/title-search/actions';
import { libraryRowPill, librarySecondary, libraryStatusText } from '@/lib/title-search/library-row';
import { addRecentSearch } from '@/lib/title-search/recent';
import { cleanQuery } from '@/lib/title-search/text';
import { searchHref } from '@/lib/title-search/url';
import { plural } from '@/lib/title/format';
import { titleHref } from '@/lib/title/href';
import { useSignOut } from './signOut';

interface PaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const ACTION_ICONS: Record<PaletteActionIcon, LucideIcon> = {
  review: ListChecks,
  find: Search,
  activity: Activity,
  library: LibraryBig,
  discover: Compass,
  trakt: Link2,
  sync: RefreshCw,
  settings: Settings,
  infuse: Tv,
  logout: LogOut,
};

const GROUPS = ['Recent', 'Your library', 'Titles', 'Actions'] as const;

/** Library titles and TMDB titles shown for a query. */
const MAX_LIBRARY = 4;
const MAX_TITLES = 5;

interface Row {
  group: (typeof GROUPS)[number];
  /** cmdk's id: unique in the list. */
  value: string;
  run: () => void;
  /** ⌘↵: Find missing or Find a torrent. */
  secondary?: { label: string; run: () => void };
  leading: ReactNode;
  label: ReactNode;
  sub: string;
  trailing?: ReactNode;
}

/**
 * ⌘K / Ctrl K / "/" (HANDOFF §3, §4.5; js/screens/search.js `MS.palette`):
 * find titles, your library and actions from anywhere.
 *
 * - Before typing: two recent searches, your four latest titles and every action.
 * - Typing: up to 4 library titles (`/library?q=`, plus TMDB titles you have),
 *   up to 5 TMDB titles you don't ("Add"), the actions whose words match, and
 *   "Search all results", always last.
 * - Keys: ↑ ↓ move (and wrap), ↵ opens, Esc closes. **⌘↵ (Ctrl ↵) runs the
 *   selected row's secondary action**: torrent search. For a library title
 *   that lacks a file it's Find missing (or Find a torrent), on the current
 *   page. For a TMDB title you don't have it's Find a torrent, on its page,
 *   where picking a torrent adds it first. Actions and recent searches have
 *   none. The footer names it when there is one.
 * - Phone: pinned to the top with Cancel; the footer and pills are hidden.
 *
 * Trakt's action is Connect Trakt (its settings) or Sync Trakt now, which
 * runs here with a toast. "Ask momoshtrem" (#9) joins when it lands.
 */
export function CommandPalette({ open, onOpenChange }: PaletteProps) {
  const router = useRouter();
  const signOut = useSignOut();
  const sheets = useSheet();
  const focus = useRestoreFocus();
  const { toast } = useToast();
  const traktSync = useTraktSync();
  const [query, setQuery] = useState('');
  const [wasOpen, setWasOpen] = useState(open);
  const [selected, setSelected] = useState('');
  const [listKey, setListKey] = useState('');
  const data = usePaletteData(query, open);

  // However it closed (Esc, outside click, ⌘K, an item), it opens empty next time
  if (open !== wasOpen) {
    setWasOpen(open);
    if (!open) setQuery('');
  }

  const typed = query.trim();
  const go = (href: string) => router.push(href);
  const find = (target: FindTarget) => (target.kind === 'sheet' ? sheets.open('torrent', target.params) : go(target.href));
  const runAction = (target: PaletteActionTarget) => {
    if (target.kind === 'href') go(target.href);
    else if (target.kind === 'sheet') sheets.open(target.name, target.params);
    else if (target.kind === 'trakt-sync') {
      traktSync.mutate(undefined, {
        onSuccess: () => toast({ tone: 'info', icon: RefreshCw, ...SYNC_STARTED_TOAST }),
        onError: (error) => toast({ tone: 'error', title: 'Couldn’t start a sync', body: error.message }),
      });
    } else void signOut();
  };
  /** Opening a title keeps the search that found it in Recent. */
  const openTitle = (href: string) => () => {
    if (data.shown && typed) addRecentSearch(data.shown.query);
    go(href);
  };

  const libraryRow = (item: LibraryItem, reason: string | null, marked: string): Row => {
    const secondary = librarySecondary(item, { torrentSheet: getSheet('torrent') !== undefined });
    const pill = libraryRowPill(item, { short: true });
    return {
      group: 'Your library',
      value: `library:${item.media_type}:${item.tmdb_id}`,
      run: openTitle(titleHref(item.media_type, item.tmdb_id)),
      secondary: secondary ? { label: secondary.label, run: () => find(secondary.target) } : undefined,
      leading: <PaletteThumb path={item.poster_path} />,
      label: <Highlight text={item.title} query={marked} />,
      sub: `${kindLine(item)} · ${reason ?? libraryStatusText(item)}`,
      trailing: <StatusPill className="p-pill" status={pill.status} label={pill.label} pct={pill.pct} />,
    };
  };
  const titleRow = (item: TitleSearchItem, marked: string): Row => ({
    group: 'Titles',
    value: `title:${item.media_type}:${item.tmdb_id}`,
    run: openTitle(titleHref(item.media_type, item.tmdb_id)),
    secondary: { label: 'Find a torrent', run: openTitle(findTorrentHref(item)) },
    leading: <PaletteThumb path={item.poster_path} />,
    label: <Highlight text={item.title} query={marked} />,
    sub: [kindLine(item), item.match_reason].filter(Boolean).join(' · '),
    trailing: (
      <span className="p-add t-small text-fg-3" aria-hidden="true">
        Add
      </span>
    ),
  });
  const actionRow = (action: PaletteAction, marked: string): Row => ({
    group: 'Actions',
    value: `action:${action.key}`,
    run: () => runAction(action.target),
    leading: <PaletteIcon icon={ACTION_ICONS[action.icon]} />,
    label: <Highlight text={action.label} query={marked} />,
    sub: action.sub,
  });

  const rows: Row[] = [];
  if (!typed) {
    for (const recent of data.recentQueries) {
      rows.push({
        group: 'Recent',
        value: `recent:${recent}`,
        run: () => {
          addRecentSearch(recent);
          go(searchHref(recent));
        },
        leading: <PaletteIcon icon={History} />,
        label: recent,
        sub: 'Search',
      });
    }
    rows.push(...data.recentTitles.map((item) => libraryRow(item, null, '')));
    rows.push(...data.actions.map((action) => actionRow(action, '')));
  } else {
    const { shown } = data;
    if (shown) {
      rows.push(...shown.sections.library.slice(0, MAX_LIBRARY).map((hit) => libraryRow(hit.item, hit.reason, shown.query)));
      rows.push(...shown.sections.titles.slice(0, MAX_TITLES).map((item) => titleRow(item, shown.query)));
    }
    rows.push(...matchingActions(data.actions, typed).map((action) => actionRow(action, typed)));
    const total = shown && shown.query === cleanQuery(typed) ? shown.sections.counts.all : 0;
    rows.push({
      group: 'Actions',
      value: 'search-all',
      run: () => {
        addRecentSearch(typed);
        go(searchHref(typed));
      },
      leading: <PaletteIcon icon={Search} />,
      label: `Search all results for “${typed}”`,
      sub: total > 0 ? plural(total, 'title') : 'Movies, shows and people',
    });
  }

  // A new list selects its first row (results arrive after typing, above the actions)
  const values = rows.map((row) => row.value).join('\n');
  if (values !== listKey) {
    setListKey(values);
    setSelected(rows[0]?.value ?? '');
  }
  const current = rows.find((row) => row.value === selected);

  const close = (then: () => void) => {
    onOpenChange(false);
    then();
  };

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'Enter' || !(event.metaKey || event.ctrlKey)) return;
    // Never the primary action: ⌘↵ on a row without a secondary does nothing
    event.preventDefault();
    if (current?.secondary) close(current.secondary.run);
  };

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim z-(--z-palette)" />
        <Dialog.Content className="palette" aria-describedby={undefined} {...focus}>
          <Dialog.Title className="sr-only">Search or jump to</Dialog.Title>
          <Command label="Search titles, your library, or run an action" shouldFilter={false} loop value={selected} onValueChange={setSelected} onKeyDown={onKeyDown}>
            <div className="p-input">
              <Search size={20} className="text-fg-3" aria-hidden="true" />
              <Command.Input
                value={query}
                onValueChange={setQuery}
                placeholder="Search or jump to…"
                autoFocus
                autoComplete="off"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                enterKeyHint="go"
              />
              <Dialog.Close className="btn btn-ghost btn-sm p-cancel">Cancel</Dialog.Close>
            </div>
            <Command.List className="p-body">
              {GROUPS.map((group) => {
                const items = rows.filter((row) => row.group === group);
                if (!items.length) return null;
                return (
                  <Command.Group key={group} heading={group}>
                    {items.map((row) => (
                      <PaletteRow key={row.value} value={row.value} onSelect={() => close(row.run)} leading={row.leading} label={row.label} sub={row.sub} trailing={row.trailing} />
                    ))}
                  </Command.Group>
                );
              })}
            </Command.List>
            <div className="p-foot" aria-hidden="true">
              <span>
                <Kbd>↑</Kbd>
                <Kbd>↓</Kbd> Navigate
              </span>
              <span>
                <Kbd>↵</Kbd> Open
              </span>
              {current?.secondary && (
                <span>
                  <Kbd>⌘↵</Kbd> {current.secondary.label}
                </span>
              )}
              <span>
                <Kbd>esc</Kbd> Close
              </span>
              <span className="ml-auto">
                <MarkMono size={14} />
                <Kbd>⌘K</Kbd> anywhere
              </span>
            </div>
          </Command>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
