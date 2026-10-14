'use client';

import { Sheet } from '@/components/ui';
import { getSheet } from '@/lib/sheets/registry';
import { useSheet } from '@/lib/sheets/useSheet';

/** Renders whichever registered sheet `?sheet=` names (Back closes it). */
export function SheetHost() {
  const { current, close, params } = useSheet();
  const definition = getSheet(current);
  if (!definition) return null;
  return (
    <Sheet
      open
      onOpenChange={(open) => !open && close(definition.params)}
      title={definition.title}
      description={definition.description}
      size={definition.size}
    >
      {definition.render(params)}
    </Sheet>
  );
}
