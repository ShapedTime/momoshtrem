'use client';

import type { ReactNode } from 'react';
import { Command } from 'cmdk';
import { CornerDownLeft, type LucideIcon } from 'lucide-react';
import { Media } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';

interface PaletteRowProps {
  /** cmdk's id for the row: unique in the list. */
  value: string;
  onSelect: () => void;
  /** A poster thumb or an icon. */
  leading: ReactNode;
  label: ReactNode;
  /** One line under the label. */
  sub: ReactNode;
  /** A library pill or "Add" (both hidden on phone). */
  trailing?: ReactNode;
}

/** One palette row (search.js `titleItem`, `actionItem`): ↵ shows on the selected one. */
export function PaletteRow({ value, onSelect, leading, label, sub, trailing }: PaletteRowProps) {
  return (
    <Command.Item value={value} onSelect={onSelect} className="p-item">
      {leading}
      <span className="grow">
        <span className="label">{label}</span>
        <span>{sub}</span>
      </span>
      {trailing}
      <CornerDownLeft size={16} className="enter" aria-hidden="true" />
    </Command.Item>
  );
}

/** A 30 px poster. */
export function PaletteThumb({ path }: { path: string | null }) {
  return <Media shape="poster" src={buildImageUrl(path, 'poster', 'small')} sizes="30px" className="thumb" />;
}

export function PaletteIcon({ icon: Icon }: { icon: LucideIcon }) {
  return (
    <span className="ic" aria-hidden="true">
      <Icon size={16} />
    </span>
  );
}
