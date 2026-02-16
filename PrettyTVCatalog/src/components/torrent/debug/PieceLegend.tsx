'use client';

const LEGEND_ITEMS = [
  { color: '#22c55e', label: 'Complete' },
  { color: '#ef4444', label: 'Now (streaming)' },
  { color: '#f97316', label: 'Next' },
  { color: '#d97706', label: 'Readahead' },
  { color: '#2563eb', label: 'Normal' },
  { color: '#1c1c1c', border: '#333', label: 'None' },
  { color: '#06b6d4', label: 'Reader position', shape: 'triangle' as const },
];

export function PieceLegend() {
  return (
    <div className="flex flex-wrap gap-4 text-sm text-text-secondary">
      {LEGEND_ITEMS.map(({ color, label, border, shape }) => (
        <div key={label} className="flex items-center gap-1.5">
          {shape === 'triangle' ? (
            <svg width="12" height="12" viewBox="0 0 12 12">
              <polygon points="6,0 12,12 0,12" fill={color} />
            </svg>
          ) : (
            <span
              className="inline-block w-3 h-3 rounded-sm"
              style={{
                backgroundColor: color,
                border: border ? `1px solid ${border}` : undefined,
              }}
            />
          )}
          <span>{label}</span>
        </div>
      ))}
    </div>
  );
}
