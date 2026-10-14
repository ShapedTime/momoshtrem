import { Tabs } from '@/components/ui';
import type { SeasonSummary } from '@/lib/api/contract';
import { seasonLabel } from '@/lib/title/episodes';

interface SeasonTabsProps {
  /** As the title lists them: Specials last. */
  seasons: SeasonSummary[];
  value: number;
  onChange: (seasonNumber: number) => void;
  /** id of the episode list the tabs control, while it's on the page. */
  controls?: string;
}

/** One tab per season with its episode count; Specials last (HANDOFF §3). */
export function SeasonTabs({ seasons, value, onChange, controls }: SeasonTabsProps) {
  return (
    <Tabs
      aria-label="Seasons"
      value={String(value)}
      onChange={(next) => onChange(Number(next))}
      tabs={seasons.map((season) => ({
        value: String(season.season_number),
        label: seasonLabel(season),
        count: season.episodes_total,
        controls,
      }))}
    />
  );
}
