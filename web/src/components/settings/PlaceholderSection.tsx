import { EmptyState, Panel } from '@/components/ui';
import type { SectionId } from './sections';

const COPY: Partial<Record<SectionId, { title: string; body: string }>> = {
  ai: {
    title: 'AI picks are on their way',
    body: 'Noir will write nightly picks from your history and answer “Ask momoshtrem”, within a monthly budget you set.',
  },
};

/** Sections whose features haven't shipped yet say so plainly. */
export function PlaceholderSection({ id }: { id: SectionId }) {
  const copy = COPY[id];
  if (!copy) return null;
  return (
    <Panel>
      <EmptyState cat="sleep" title={copy.title} headingLevel="h3">
        {copy.body}
      </EmptyState>
    </Panel>
  );
}
