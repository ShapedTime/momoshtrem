'use client';

import { useId } from 'react';
import { Check } from 'lucide-react';
import { MEMBER_COLORS, type MemberColor } from '@/lib/api/contract';
import { MEMBER_COLOR_LABELS } from '@/lib/members/display';

interface ColourFieldProps {
  value: MemberColor;
  onChange: (color: MemberColor) => void;
}

/**
 * A member's colour (#247): the six identity colours as a radio group, a
 * 32 px disc in a 44 px hit area each. The chosen one is ringed and ticked,
 * and its name shows beside the label, so it never rests on colour alone.
 */
export function ColourField({ value, onChange }: ColourFieldProps) {
  const id = useId();
  return (
    <div className="field">
      <div className="flex items-baseline justify-between gap-2">
        <span id={`${id}-label`} className="label">
          Colour
        </span>
        <span className="t-small text-fg-3" aria-hidden="true">
          {MEMBER_COLOR_LABELS[value]}
        </span>
      </div>
      <div role="radiogroup" aria-labelledby={`${id}-label`} className="set-swatches">
        {MEMBER_COLORS.map((color) => (
          <label key={color} className="set-swatch" data-color={color}>
            <input type="radio" name={`${id}-colour`} value={color} checked={value === color} onChange={() => onChange(color)} />
            <span className="disc" aria-hidden="true">
              <Check size={18} strokeWidth={2.75} />
            </span>
            <span className="sr-only">{MEMBER_COLOR_LABELS[color]}</span>
          </label>
        ))}
      </div>
    </div>
  );
}
