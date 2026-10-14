import type { ReactNode } from 'react';
import type { SheetProps } from '@/components/ui';
import { InfuseSetupSheet } from '@/components/shell/InfuseSetupSheet';

export interface SheetDefinition {
  title: string;
  description?: string;
  size?: SheetProps['size'];
  /** Other URL params this sheet owns; removed when it closes from a deep link. */
  params?: string[];
  render: (params: URLSearchParams) => ReactNode;
}

/**
 * Sheets that open from the URL (`?sheet=<name>`). Feature issues register
 * theirs here: `torrent` (#6), `inspect` (#6), `episode` (#4), …
 */
export const SHEETS: Record<string, SheetDefinition> = {
  'infuse-setup': {
    title: 'Play it in Infuse',
    description: 'momoshtrem gets things ready; Infuse on your Apple TV streams them over WebDAV.',
    size: 'narrow',
    render: () => <InfuseSetupSheet />,
  },
};
