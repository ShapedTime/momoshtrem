'use client';

import { useCallback } from 'react';
import { SUBTITLE_LANGUAGES, type SubtitleLanguageCode } from '@/types/subtitle';

interface LanguageSelectorProps {
  selected: SubtitleLanguageCode[];
  onChange: (selected: SubtitleLanguageCode[]) => void;
}

export function LanguageSelector({ selected, onChange }: LanguageSelectorProps) {
  const toggleLanguage = useCallback(
    (code: SubtitleLanguageCode) => {
      if (selected.includes(code)) {
        onChange(selected.filter((c) => c !== code));
      } else {
        onChange([...selected, code]);
      }
    },
    [selected, onChange]
  );

  const selectAll = useCallback(() => {
    onChange(SUBTITLE_LANGUAGES.map((l) => l.code));
  }, [onChange]);

  const clearAll = useCallback(() => {
    onChange([]);
  }, [onChange]);

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm text-text-secondary">Languages</span>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={selectAll}
            className="text-xs text-accent-blue hover:text-accent-blue-hover transition-colors"
          >
            Select All
          </button>
          <span className="text-text-muted">|</span>
          <button
            type="button"
            onClick={clearAll}
            className="text-xs text-accent-blue hover:text-accent-blue-hover transition-colors"
          >
            Clear
          </button>
        </div>
      </div>

      <div className="flex flex-wrap gap-2">
        {SUBTITLE_LANGUAGES.map((lang) => {
          const isSelected = selected.includes(lang.code);
          return (
            <button
              key={lang.code}
              type="button"
              onClick={() => toggleLanguage(lang.code)}
              className={`
                px-3 py-1.5 rounded-full text-sm font-medium transition-colors
                ${
                  isSelected
                    ? 'bg-accent-blue text-white'
                    : 'bg-bg-secondary text-text-secondary hover:bg-bg-tertiary hover:text-white'
                }
              `}
            >
              {lang.name}
            </button>
          );
        })}
      </div>
    </div>
  );
}
