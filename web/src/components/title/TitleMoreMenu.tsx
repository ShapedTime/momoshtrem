'use client';

import { useState } from 'react';
import { Ellipsis, ExternalLink, HardDriveDownload, type LucideIcon, MonitorPlay, Pin, PinOff, RefreshCw, Trash2 } from 'lucide-react';
import { Button, IconButton, Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger, Sheet } from '@/components/ui';
import { DESKTOP_QUERY, useMediaQuery } from '@/hooks/useMediaQuery';
import type { Title } from '@/lib/api/contract';
import { keepOf } from '@/lib/title/keep';
import { downloadsAvailable } from '@/lib/title/keep-sheet';

interface TitleMoreMenuProps {
  title: Title;
  /** In the phone bar (glass, 44 px) or beside the eyebrow on desktop (36 px). */
  placement: 'phone' | 'desktop';
  onRefresh: () => void;
  /** Asks for confirmation itself (useConfirm). */
  onRemove: () => void;
  /** Opens the HDRezka sheet; given only while HDRezka is set up and the title can use it. */
  onStreamFromHDRezka?: () => void;
  /** Opens the Keep sheet (#206): Download & keep, or Change duration once it's kept. */
  onKeep?: () => void;
  /** Stop keeping the title's own download; asks for confirmation itself. */
  onStopKeeping?: () => void;
}

const tmdbUrl = (title: Title) => `https://www.themoviedb.org/${title.media_type}/${title.tmdb_id}`;
const traktUrl = (title: Title) => `https://trakt.tv/search/tmdb/${title.tmdb_id}?id_type=${title.media_type === 'movie' ? 'movie' : 'show'}`;

export interface MenuAction {
  key: string;
  label: string;
  Icon: LucideIcon;
  run: () => void;
}

/**
 * Download & keep, or Change duration and Stop keeping: the movie's download, or the show's own
 * (show scope). Nothing while the backend has no downloads (no `keep` field), and no new Keep for a
 * movie that streams from HDRezka (it would be refused); one kept before can still change or stop.
 */
export function keepActions(title: Title, onKeep?: () => void, onStopKeeping?: () => void): MenuAction[] {
  if (!title.library.in_library || !onKeep || !downloadsAvailable(title.library)) return [];
  const show = title.media_type === 'tv';
  if (keepOf(title.library.keep)?.reason !== 'kept') {
    if (title.media_type === 'movie' && title.library.stream) return [];
    return [{ key: 'keep', label: show ? 'Download & keep show…' : 'Download & keep…', Icon: HardDriveDownload, run: onKeep }];
  }
  return [
    { key: 'change', label: 'Change duration…', Icon: Pin, run: onKeep },
    ...(onStopKeeping ? [{ key: 'stop', label: show ? 'Stop keeping show…' : 'Stop keeping…', Icon: PinOff, run: onStopKeeping }] : []),
  ];
}

/**
 * The title's ⋯ menu: Stream from HDRezka (when set up), Download & keep (or
 * Change duration and Stop keeping), Refresh from TMDB, Open on TMDB / Trakt,
 * and Remove from library (quiet, confirmed). A popover on desktop, a sheet on phone.
 */
export function TitleMoreMenu({ title, placement, onRefresh, onRemove, onStreamFromHDRezka, onKeep, onStopKeeping }: TitleMoreMenuProps) {
  const desktop = useMediaQuery(DESKTOP_QUERY);
  const [sheetOpen, setSheetOpen] = useState(false);
  const inLibrary = title.library.in_library;
  const hdrezka = inLibrary ? onStreamFromHDRezka : undefined;
  const keeps = keepActions(title, onKeep, onStopKeeping);
  const trigger = {
    variant: 'glass' as const,
    size: placement === 'desktop' ? ('sm' as const) : ('md' as const),
    'aria-label': `More actions for ${title.title}`,
  };
  const icon = <Ellipsis size={20} aria-hidden="true" />;

  if (desktop) {
    return (
      <Menu>
        <MenuTrigger asChild>
          <IconButton {...trigger}>{icon}</IconButton>
        </MenuTrigger>
        <MenuContent aria-label="More actions">
          {hdrezka && (
            <MenuItem icon={<MonitorPlay size={18} aria-hidden="true" />} onSelect={() => window.setTimeout(hdrezka, 0)}>
              Stream from HDRezka…
            </MenuItem>
          )}
          {/* After the menu has closed, so the sheet or the confirm dialog takes focus */}
          {keeps.map((action) => (
            <MenuItem key={action.key} icon={<action.Icon size={18} aria-hidden="true" />} onSelect={() => window.setTimeout(action.run, 0)}>
              {action.label}
            </MenuItem>
          ))}
          {inLibrary && (
            <MenuItem icon={<RefreshCw size={18} aria-hidden="true" />} onSelect={onRefresh}>
              Refresh from TMDB
            </MenuItem>
          )}
          <MenuItem icon={<ExternalLink size={18} aria-hidden="true" />} href={tmdbUrl(title)} external>
            Open on TMDB
          </MenuItem>
          <MenuItem icon={<ExternalLink size={18} aria-hidden="true" />} href={traktUrl(title)} external>
            Open on Trakt
          </MenuItem>
          {inLibrary && (
            <>
              <MenuSeparator />
              {/* After the menu has closed, so the confirm dialog takes focus and the page stays clickable */}
              <MenuItem danger icon={<Trash2 size={18} aria-hidden="true" />} onSelect={() => window.setTimeout(onRemove, 0)}>
                Remove from library…
              </MenuItem>
            </>
          )}
        </MenuContent>
      </Menu>
    );
  }

  const run = (action: () => void) => {
    setSheetOpen(false);
    window.setTimeout(action, 0);
  };
  return (
    <>
      <IconButton {...trigger} aria-haspopup="dialog" onClick={() => setSheetOpen(true)}>
        {icon}
      </IconButton>
      <Sheet open={sheetOpen} onOpenChange={setSheetOpen} title="More" size="narrow">
        <div className="flex flex-col gap-1">
          {hdrezka && (
            <button type="button" className="list-row rounded-md text-left" onClick={() => run(hdrezka)}>
              <MonitorPlay size={20} className="text-fg-3" aria-hidden="true" />
              <span className="grow font-medium">Stream from HDRezka…</span>
            </button>
          )}
          {keeps.map((action) => (
            <button key={action.key} type="button" className="list-row rounded-md text-left" onClick={() => run(action.run)}>
              <action.Icon size={20} className="text-fg-3" aria-hidden="true" />
              <span className="grow font-medium">{action.label}</span>
            </button>
          ))}
          {inLibrary && (
            <button type="button" className="list-row rounded-md text-left" onClick={() => run(onRefresh)}>
              <RefreshCw size={20} className="text-fg-3" aria-hidden="true" />
              <span className="grow font-medium">Refresh from TMDB</span>
            </button>
          )}
          <a href={tmdbUrl(title)} target="_blank" rel="noopener noreferrer" className="list-row rounded-md" onClick={() => setSheetOpen(false)}>
            <ExternalLink size={20} className="text-fg-3" aria-hidden="true" />
            <span className="grow font-medium">Open on TMDB</span>
          </a>
          <a href={traktUrl(title)} target="_blank" rel="noopener noreferrer" className="list-row rounded-md" onClick={() => setSheetOpen(false)}>
            <ExternalLink size={20} className="text-fg-3" aria-hidden="true" />
            <span className="grow font-medium">Open on Trakt</span>
          </a>
        </div>
        {inLibrary && (
          <Button variant="danger-quiet" block className="mt-4" onClick={() => run(onRemove)}>
            <Trash2 size={18} aria-hidden="true" /> Remove from library…
          </Button>
        )}
      </Sheet>
    </>
  );
}
