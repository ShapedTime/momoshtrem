import type { ComponentType, ReactNode } from 'react';
import type { SheetProps } from '@/components/ui';
import { InfuseSetupSheet } from '@/components/shell/InfuseSetupSheet';

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
 * theirs here: `torrent` (#6), `inspect` (#6), `episode` (#4), …
 */
export const SHEETS: Record<string, SheetDefinition> = {
  'infuse-setup': {
    title: 'Play it in Infuse',
    description: 'momoshtrem gets things ready; Infuse on your Apple TV streams them over WebDAV.',
    size: 'narrow',
    // `path`: a title's WebDAV path, for a "find it here" step
    params: ['path'],
    render: (params) => <InfuseSetupSheet path={params.get('path')} />,
  },
};

/** The sheet registered as `name`. Own keys only, so `?sheet=toString` opens nothing. */
export function getSheet(name: string | null): SheetDefinition | undefined {
  return name !== null && Object.hasOwn(SHEETS, name) ? SHEETS[name] : undefined;
}
