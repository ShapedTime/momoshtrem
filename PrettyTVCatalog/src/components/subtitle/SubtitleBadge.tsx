'use client';

interface SubtitleBadgeProps {
  count: number;
  className?: string;
}

function SubtitleIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <rect x="2" y="4" width="20" height="16" rx="2" />
      <line x1="6" y1="12" x2="18" y2="12" />
      <line x1="6" y1="16" x2="14" y2="16" />
    </svg>
  );
}

export function SubtitleBadge({ count, className = '' }: SubtitleBadgeProps) {
  if (count === 0) return null;

  return (
    <div
      className={`
        inline-flex items-center gap-1 px-2 py-0.5
        bg-accent-blue/20 text-accent-blue
        rounded text-xs font-medium
        ${className}
      `}
      title={`${count} subtitle${count !== 1 ? 's' : ''} available`}
    >
      <SubtitleIcon />
      <span>{count}</span>
    </div>
  );
}
