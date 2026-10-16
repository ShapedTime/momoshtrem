import type { ComponentType, ReactNode } from 'react';
import type { SheetProps } from '@/components/ui';
import { HDRezkaSheet } from '@/components/hdrezka/HDRezkaSheet';
import { TorrentSearchSheet } from '@/components/search/TorrentSearchSheet';
import { InfuseSetupSheet } from '@/components/shell/InfuseSetupSheet';
import { EpisodeSheet } from '@/components/title/show/EpisodeSheet';
import { ShowSettingsSheet } from '@/components/title/show/ShowSettingsSheet';
import { SOURCE_PARAMS } from '@/lib/source/context';

interface SheetBase {
  /** Other URL params this sheet owns; removed when it closes from a deep link. */
  params?: string[];
}

/** The host draws the <Sheet> (head, body) around what `render` returns. */
interface ContentSheet extends SheetBase {
  title: string;
  description?: string;
  size?: SheetProps['size'];
  render: (params: URLSearchParams) => ReactNode;
  Component?: undefined;
}

export interface SheetComponentProps {
  params: URLSearchParams;
  /** Closes the sheet and drops its params: `<Sheet open onOpenChange={(open) => !open && close()}>`. */
  close: () => void;
}

/**
 * The sheet draws its own <Sheet>, for sheets that own their footer or
 * state (a form with a sticky Save, an episode's actions).
 */
interface SelfRenderingSheet extends SheetBase {
  Component: ComponentType<SheetComponentProps>;
  title?: undefined;
  render?: undefined;
}

export type SheetDefinition = ContentSheet | SelfRenderingSheet;

/**
 * Sheets that open from the URL (`?sheet=<name>`). Feature issues register
 * theirs here: `torrent` (#6), `episode` (#4), … (Inspect is a sheet nested in `torrent`, not a URL sheet.)
 * `torrent` and `hdrezka` are the source sheet's tabs: same params, same head
 * (`components/source`), swapped with `useSheet().swap`.
 */
export const SHEETS: Record<string, SheetDefinition> = {
  'infuse-setup': {
    title: 'Play it in Infuse',
    description: 'momoshtrem gets things ready; Infuse on your Apple TV streams them over WebDAV.',
    size: 'narrow',
    // A title's WebDAV path and URL, for a "find it here" step (infuseSetupParams)
    params: ['path', 'url'],
    render: (params) => <InfuseSetupSheet path={params.get('path')} url={params.get('url')} />,
  },
  // A show's episode: `tmdb` (the show) and `se` ("S01E05"); `season` is the page's tab and stays
  episode: { params: ['tmdb', 'se'], Component: EpisodeSheet },
  'show-settings': { params: ['tmdb'], Component: ShowSettingsSheet },
  // Find a source, HDRezka tab (#67): `movie`, or `show` with `season` (the page's tab, so it stays) and `se`
  hdrezka: { params: SOURCE_PARAMS, Component: HDRezkaSheet },
  // Find a source, Torrents tab (#92), on the same params; nothing links here until TORRENT_SHEET_LIVE (#95)
  torrent: { params: SOURCE_PARAMS, Component: TorrentSearchSheet },
};

/** The sheet registered as `name`. Own keys only, so `?sheet=toString` opens nothing. */
export function getSheet(name: string | null): SheetDefinition | undefined {
  return name !== null && Object.hasOwn(SHEETS, name) ? SHEETS[name] : undefined;
}
