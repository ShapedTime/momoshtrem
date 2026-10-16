import { HardDrive, Link2, type LucideIcon, Server, SlidersHorizontal, Sparkles, Tv } from 'lucide-react';

export type SectionId = 'trakt' | 'ai' | 'quality' | 'playback' | 'storage' | 'status';

export interface SettingsSection {
  id: SectionId;
  label: string;
  group: 'Integrations' | 'Preferences' | 'Playback' | 'System';
  icon: LucideIcon;
  description: string;
}

/** Settings sections (screens/settings). */
export const SECTIONS: SettingsSection[] = [
  { id: 'trakt', label: 'Trakt', group: 'Integrations', icon: Link2, description: 'Your watch history powers Up next and personal picks.' },
  { id: 'ai', label: 'AI & picks', group: 'Preferences', icon: Sparkles, description: 'When Noir writes your picks, which model it uses, and what it may spend.' },
  { id: 'quality', label: 'Quality profile', group: 'Preferences', icon: SlidersHorizontal, description: 'What “Best match” means when you search for a torrent.' },
  { id: 'playback', label: 'Infuse & WebDAV', group: 'Playback', icon: Tv, description: 'How Infuse on your Apple TV reaches momoshtrem.' },
  { id: 'storage', label: 'Storage', group: 'Playback', icon: HardDrive, description: 'How much of the disk momoshtrem may use, and what it downloads in the background.' },
  { id: 'status', label: 'System status', group: 'System', icon: Server, description: 'Every service momoshtrem depends on, checked just now.' },
];

export const DEFAULT_SECTION: SectionId = 'playback';

export function isSectionId(value: string | null): value is SectionId {
  return SECTIONS.some((section) => section.id === value);
}
