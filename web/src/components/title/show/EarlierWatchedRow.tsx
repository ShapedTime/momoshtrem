import { ChevronUp } from 'lucide-react';
import { Button } from '@/components/ui';

/** Fully watched leading episodes, collapsed (HANDOFF §5.4): one row that brings them back in place. */
export function EarlierWatchedRow({ from, to, onExpand }: { from: number; to: number; onExpand: () => void }) {
  return (
    <Button variant="ghost" block className="show-earlier" onClick={onExpand} aria-expanded={false}>
      <ChevronUp size={16} aria-hidden="true" /> Show episodes {from}–{to} (all watched)
    </Button>
  );
}
