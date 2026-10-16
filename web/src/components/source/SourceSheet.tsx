'use client';

import { type ReactNode, useCallback, useId } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Sheet, Tabs } from '@/components/ui';
import { useHDRezkaEnabled } from '@/hooks/useHDRezka';
import { openLegacySearch } from '@/hooks/useLegacySearch';
import type { Title } from '@/lib/api/contract';
import { seasonQuery, titleQuery } from '@/lib/api/endpoints';
import { TORRENT_SHEET_LIVE } from '@/lib/search/live';
import { getSheet } from '@/lib/sheets/registry';
import { useSheet } from '@/lib/sheets/useSheet';
import { contextMediaType, legacyScope, type SourceContext, sourceHeading } from '@/lib/source/context';
import { legacySearchContext } from '@/lib/title/legacy-search';

/** The source sheet's tabs; each is its own registered sheet (`?sheet=torrent|hdrezka`) on the same params. */
export type SourceTab = 'torrent' | 'hdrezka';

const TAB_LABEL: Record<SourceTab, string> = { torrent: 'Torrents', hdrezka: 'HDRezka' };

/** The title (cached by the page) and, for an episode, its row: what the head names. */
export function useSourceTitle(context: SourceContext) {
  const title = useQuery(titleQuery(contextMediaType(context), context.tmdbId)).data;
  const inLibrary = Boolean(title?.library.in_library);
  const episodeContext = context.kind === 'show' && context.season !== null && context.episode !== null ? context : null;
  const season = useQuery({ ...seasonQuery(context.tmdbId, episodeContext?.season ?? 0), enabled: episodeContext !== null && inLibrary }).data;
  const episode = episodeContext ? season?.episodes.find((e) => e.episode_number === episodeContext.episode) : undefined;
  return { title, episode };
}

/**
 * Switch the source sheet to another tab. A registered sibling swaps in on the
 * same params (Back still closes it). Until `TORRENT_SHEET_LIVE` (#95),
 * Torrents closes the sheet and opens the legacy search for the same context,
 * once the sheet is gone (a sheet traps focus).
 */
export function useSwitchSource(context: SourceContext, close: () => void, title: Title | undefined) {
  const sheets = useSheet();
  return useCallback(
    (tab: SourceTab) => {
      if (getSheet(tab) && (tab !== 'torrent' || TORRENT_SHEET_LIVE)) {
        sheets.swap(tab);
        return;
      }
      close();
      if (tab === 'torrent' && title) openLegacySearch(legacySearchContext(title, legacyScope(context)));
    },
    [close, context, sheets, title]
  );
}

interface SourceSheetProps {
  tab: SourceTab;
  context: SourceContext;
  /** Closes the sheet and drops its params (`SheetComponentProps.close`). */
  close: () => void;
  /** Under the tabs: the tab's search field and chips. */
  head?: ReactNode;
  /** The tab's sticky primary action. */
  footer?: ReactNode;
  children: ReactNode;
}

/**
 * The shared head of the source sheet (#67, and #6's Torrents tab):
 * "Find a source · {title}", the context ("2014 · 2h 49m", "Season 1",
 * "S01E29 · name"), a close that keeps the tab's choices, and the
 * Torrents · HDRezka tabs. Full height from the first frame, so loading never
 * moves it.
 */
export function SourceSheet({ tab, context, close, head, footer, children }: SourceSheetProps) {
  const panelId = useId();
  const { title, episode } = useSourceTitle(context);
  const hdrezka = useHDRezkaEnabled();
  const switchTo = useSwitchSource(context, close, title);
  const { label, heading } = sourceHeading(context, title, episode);
  // HDRezka is opt-in: without it, the Torrents tab stands alone
  const tabs: SourceTab[] = hdrezka || tab === 'hdrezka' ? ['torrent', 'hdrezka'] : ['torrent'];

  return (
    <Sheet
      open
      onOpenChange={(open) => !open && close()}
      size="full"
      className="source-sheet"
      kicker={label ? `Find a source · ${label}` : 'Find a source'}
      title={
        <span className="block truncate" title={heading}>
          {heading}
        </span>
      }
      closeLabel="Close (your choices are kept)"
      head={
        <>
          {tabs.length > 1 && (
            <Tabs
              aria-label="Source"
              value={tab}
              onChange={(next) => next !== tab && switchTo(next)}
              tabs={tabs.map((value) => ({ value, label: TAB_LABEL[value], controls: value === tab ? panelId : undefined }))}
            />
          )}
          {head}
        </>
      }
      footer={footer}
    >
      <div id={panelId} className="flex flex-col gap-4" {...(tabs.length > 1 ? { role: 'tabpanel', 'aria-label': TAB_LABEL[tab] } : {})}>
        {children}
      </div>
    </Sheet>
  );
}
