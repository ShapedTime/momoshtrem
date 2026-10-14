'use client';

import { Sheet } from '@/components/ui';
import { getSheet } from '@/lib/sheets/registry';
import { useSheet } from '@/lib/sheets/useSheet';

/** Renders whichever registered sheet `?sheet=` names (Back closes it). */
export function SheetHost() {
  const { current, close, params } = useSheet();
  const definition = getSheet(current);
  if (!definition) return null;
  const closeSheet = () => close(definition.params);
  if (definition.Component) {
    const { Component } = definition;
    return <Component params={params} close={closeSheet} />;
  }
  return (
    <Sheet
      open
      onOpenChange={(open) => !open && closeSheet()}
      title={definition.title}
      description={definition.description}
      size={definition.size}
    >
      {definition.render(params)}
    </Sheet>
  );
}
