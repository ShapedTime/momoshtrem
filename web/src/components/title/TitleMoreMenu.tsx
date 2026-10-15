'use client';

import { useState } from 'react';
import { Ellipsis, ExternalLink, MonitorPlay, RefreshCw, Trash2 } from 'lucide-react';
import { Button, IconButton, Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger, Sheet } from '@/components/ui';
import { DESKTOP_QUERY, useMediaQuery } from '@/hooks/useMediaQuery';
import type { Title } from '@/lib/api/contract';

interface TitleMoreMenuProps {
  title: Title;
  /** In the phone bar (glass, 44 px) or beside the eyebrow on desktop (36 px). */
  placement: 'phone' | 'desktop';
  onRefresh: () => void;
  /** Asks for confirmation itself (useConfirm). */
  onRemove: () => void;
  /** Opens the HDRezka sheet; given only while HDRezka is set up and the title can use it. */
  onStreamFromHDRezka?: () => void;
}

const tmdbUrl = (title: Title) => `https://www.themoviedb.org/${title.media_type}/${title.tmdb_id}`;
const traktUrl = (title: Title) => `https://trakt.tv/search/tmdb/${title.tmdb_id}?id_type=${title.media_type === 'movie' ? 'movie' : 'show'}`;

/**
 * The title's ⋯ menu: Stream from HDRezka (when set up), Refresh from TMDB,
 * Open on TMDB / Trakt, and Remove from library (quiet, confirmed). A
 * popover on desktop, a sheet on phone.
 */
export function TitleMoreMenu({ title, placement, onRefresh, onRemove, onStreamFromHDRezka }: TitleMoreMenuProps) {
  const desktop = useMediaQuery(DESKTOP_QUERY);
  const [sheetOpen, setSheetOpen] = useState(false);
  const inLibrary = title.library.in_library;
  const hdrezka = inLibrary ? onStreamFromHDRezka : undefined;
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
