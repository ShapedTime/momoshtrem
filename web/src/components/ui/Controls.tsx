'use client';

import { type KeyboardEvent, type ReactNode, useRef } from 'react';
import { X } from 'lucide-react';
import { cx } from '@/lib/utils/cx';

/* ---- Switch ------------------------------------------------------------- */

interface SwitchProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  /** Accessible name when there's no visible <label htmlFor>. */
  label?: string;
  /** id of a hint that describes the setting. */
  describedBy?: string;
  id?: string;
  disabled?: boolean;
}

export function Switch({ checked, onCheckedChange, label, describedBy, id, disabled }: SwitchProps) {
  return (
    <span className="switch">
      <input
        type="checkbox"
        role="switch"
        id={id}
        checked={checked}
        disabled={disabled}
        aria-label={label}
        aria-describedby={describedBy}
        onChange={(event) => onCheckedChange(event.target.checked)}
      />
      <span className="track" aria-hidden="true" />
      <span className="thumb" aria-hidden="true" />
    </span>
  );
}

/* ---- Chip (static attribute) -------------------------------------------- */

export type ChipTone = 'default' | 'hi' | 'accent' | 'pack' | 'warn' | 'err' | 'glass';

/** Small static tag for parsed attributes (2160p, DV, x265, Season pack). */
export function Chip({ tone = 'default', className, children }: { tone?: ChipTone; className?: string; children: ReactNode }) {
  return <span className={cx('chip', tone !== 'default' && `chip--${tone}`, className)}>{children}</span>;
}

/* ---- FilterChip (toggle) ------------------------------------------------ */

interface FilterChipProps {
  pressed: boolean;
  onPressedChange?: (pressed: boolean) => void;
  /** Facet count; shown always, dimmed at zero. */
  count?: number;
  /** Renders a remove affordance; `onRemove` handles it. */
  onRemove?: () => void;
  /** Opens a menu or nested sheet instead of toggling. */
  onOpenMenu?: () => void;
  disabled?: boolean;
  className?: string;
  children: ReactNode;
}

export function FilterChip({ pressed, onPressedChange, count, onRemove, onOpenMenu, disabled, className, children }: FilterChipProps) {
  const zero = count === 0 && !pressed;
  return (
    <button
      type="button"
      className={cx('fchip', onOpenMenu && 'fchip--menu', zero && 'is-zero', className)}
      aria-pressed={onOpenMenu ? undefined : pressed}
      aria-haspopup={onOpenMenu ? 'dialog' : undefined}
      disabled={disabled}
      onClick={() => {
        if (onOpenMenu) onOpenMenu();
        else if (onRemove && pressed) onRemove();
        else onPressedChange?.(!pressed);
      }}
    >
      {children}
      {count !== undefined && <span className="n">{count}</span>}
      {onRemove && pressed && (
        <span className="x" aria-hidden="true">
          <X size={12} strokeWidth={2.4} />
        </span>
      )}
    </button>
  );
}

/* ---- SegmentedControl (modes of the same content) ----------------------- */

export interface SegmentOption<T extends string> {
  value: T;
  label: ReactNode;
  /** Accessible name when `label` is an icon. */
  ariaLabel?: string;
}

interface SegmentedControlProps<T extends string> {
  options: SegmentOption<T>[];
  value: T;
  onChange: (value: T) => void;
  'aria-label': string;
  block?: boolean;
  className?: string;
}

export function SegmentedControl<T extends string>({ options, value, onChange, block, className, ...rest }: SegmentedControlProps<T>) {
  return (
    <div role="group" aria-label={rest['aria-label']} className={cx('seg', block ? 'seg--block' : 'w-fit', className)}>
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={option.value === value}
          aria-label={option.ariaLabel}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

/* ---- Tabs (sibling content) --------------------------------------------- */

export interface TabItem<T extends string> {
  value: T;
  label: ReactNode;
  count?: number;
  /** id of the panel this tab controls. */
  controls?: string;
}

interface TabsProps<T extends string> {
  tabs: TabItem<T>[];
  value: T;
  onChange: (value: T) => void;
  'aria-label': string;
  className?: string;
}

/** Tab list with roving focus (← → Home End). Render the active panel yourself. */
export function Tabs<T extends string>({ tabs, value, onChange, className, ...rest }: TabsProps<T>) {
  const listRef = useRef<HTMLDivElement>(null);

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const index = tabs.findIndex((tab) => tab.value === value);
    const last = tabs.length - 1;
    const next =
      event.key === 'ArrowRight' ? (index === last ? 0 : index + 1)
      : event.key === 'ArrowLeft' ? (index === 0 ? last : index - 1)
      : event.key === 'Home' ? 0
      : event.key === 'End' ? last
      : null;
    if (next === null) return;
    event.preventDefault();
    onChange(tabs[next].value);
    listRef.current?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus();
  }

  return (
    <div ref={listRef} role="tablist" aria-label={rest['aria-label']} className={cx('tabs', className)} onKeyDown={onKeyDown}>
      {tabs.map((tab) => {
        const selected = tab.value === value;
        return (
          <button
            key={tab.value}
            type="button"
            role="tab"
            aria-selected={selected}
            aria-controls={tab.controls}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(tab.value)}
          >
            {tab.label}
            {tab.count !== undefined && <span className="n">{tab.count}</span>}
          </button>
        );
      })}
    </div>
  );
}
