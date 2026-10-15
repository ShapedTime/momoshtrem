'use client';

import { useState } from 'react';
import { MonitorPlay, RefreshCw, Search } from 'lucide-react';
import { Button, Menu, MenuContent, MenuItem, MenuTrigger, Sheet } from '@/components/ui';
import { DESKTOP_QUERY, useMediaQuery } from '@/hooks/useMediaQuery';

interface ReplaceMenuProps {
  onFindTorrent: () => void;
  onStreamFromHDRezka: () => void;
  /** Inside another sheet (the episode sheet): the phone menu stacks on it. */
  nested?: boolean;
}

/**
 * Replace in the current-file card, once there are two sources (#63):
 * Find a torrent or Stream from HDRezka. A popover on desktop, a sheet on
 * phone. Each runs after the menu has closed, so what it opens takes focus.
 */
export function ReplaceMenu({ onFindTorrent, onStreamFromHDRezka, nested = false }: ReplaceMenuProps) {
  const desktop = useMediaQuery(DESKTOP_QUERY);
  const [sheetOpen, setSheetOpen] = useState(false);
  const trigger = (
    <>
      <RefreshCw size={15} aria-hidden="true" /> Replace
    </>
  );
  const later = (action: () => void) => () => window.setTimeout(action, 0);

  if (desktop) {
    return (
      <Menu>
        <MenuTrigger asChild>
          <Button size="sm">{trigger}</Button>
        </MenuTrigger>
        <MenuContent align="start" aria-label="Replace with">
          <MenuItem icon={<Search size={18} aria-hidden="true" />} onSelect={later(onFindTorrent)}>
            Find a torrent
          </MenuItem>
          <MenuItem icon={<MonitorPlay size={18} aria-hidden="true" />} onSelect={later(onStreamFromHDRezka)}>
            Stream from HDRezka
          </MenuItem>
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
      <Button size="sm" aria-haspopup="dialog" onClick={() => setSheetOpen(true)}>
        {trigger}
      </Button>
      <Sheet open={sheetOpen} onOpenChange={setSheetOpen} title="Replace with" size="narrow" nested={nested}>
        <div className="flex flex-col gap-1">
          <button type="button" className="list-row rounded-md text-left" onClick={() => run(onFindTorrent)}>
            <Search size={20} className="text-fg-3" aria-hidden="true" />
            <span className="grow font-medium">Find a torrent</span>
          </button>
          <button type="button" className="list-row rounded-md text-left" onClick={() => run(onStreamFromHDRezka)}>
            <MonitorPlay size={20} className="text-fg-3" aria-hidden="true" />
            <span className="grow font-medium">Stream from HDRezka</span>
          </button>
        </div>
      </Sheet>
    </>
  );
}
