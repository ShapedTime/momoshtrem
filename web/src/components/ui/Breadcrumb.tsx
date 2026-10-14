import Link from 'next/link';
import { ChevronRightIcon } from './Icons';

export interface BreadcrumbItem {
  label: string;
  href?: string;
}

interface BreadcrumbProps {
  items: BreadcrumbItem[];
  className?: string;
}

/**
 * Breadcrumb navigation component.
 * Shows hierarchical path with clickable links to parent sections.
 * Last item (current page) is not a link.
 */
export function Breadcrumb({ items, className = '' }: BreadcrumbProps) {
  if (items.length === 0) return null;

  return (
    <nav
      aria-label="Breadcrumb"
      className={`px-4 sm:px-6 lg:px-12 py-3 ${className}`}
    >
      <ol className="flex items-center gap-1 sm:gap-2 text-sm max-w-screen-2xl mx-auto">
        {items.map((item, index) => {
          const isLast = index === items.length - 1;

          return (
            <li key={index} className="flex items-center gap-1 sm:gap-2 min-w-0">
              {index > 0 && (
                <ChevronRightIcon
                  size={16}
                  className="text-text-muted flex-shrink-0"
                />
              )}
              {item.href && !isLast ? (
                <Link
                  href={item.href}
                  className="
                    text-text-secondary hover:text-white
                    transition-colors
                    focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
                    rounded-sm
                  "
                >
                  {item.label}
                </Link>
              ) : (
                <span
                  className="text-white font-medium truncate max-w-[200px] sm:max-w-[300px]"
                  aria-current={isLast ? 'page' : undefined}
                  title={item.label}
                >
                  {item.label}
                </span>
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
