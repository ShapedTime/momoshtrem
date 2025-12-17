import { HTMLAttributes } from 'react';

type SkeletonVariant = 'rectangle' | 'circle';

interface SkeletonProps extends HTMLAttributes<HTMLDivElement> {
  variant?: SkeletonVariant;
  width?: string | number;
  height?: string | number;
}

export function Skeleton({
  variant = 'rectangle',
  width,
  height,
  className = '',
  style,
  ...props
}: SkeletonProps) {
  const baseStyles = 'bg-bg-hover animate-pulse motion-reduce:animate-none';
  const variantStyles = variant === 'circle' ? 'rounded-full' : 'rounded-md';

  const dimensions: React.CSSProperties = {
    width: typeof width === 'number' ? `${width}px` : width,
    height: typeof height === 'number' ? `${height}px` : height,
    ...style,
  };

  return (
    <div
      className={`${baseStyles} ${variantStyles} ${className}`}
      style={dimensions}
      aria-hidden="true"
      {...props}
    />
  );
}

export function SkeletonText({
  lines = 1,
  className = '',
}: {
  lines?: number;
  className?: string;
}) {
  return (
    <div className={`space-y-2 ${className}`}>
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton
          key={i}
          height={16}
          className={i === lines - 1 && lines > 1 ? 'w-3/4' : 'w-full'}
        />
      ))}
    </div>
  );
}
