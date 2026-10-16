'use client';

import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { ChevronLeft, ChevronRight, LogOut } from 'lucide-react';
import { useSignOut } from '@/components/shell/signOut';
import { qualityProfileQuery } from '@/lib/api/endpoints/search';
import { statusQuery } from '@/lib/api/endpoints/system';
import { traktIntegrationQuery } from '@/lib/api/endpoints/trakt';
import type { QualityProfile, SystemStatus, TraktIntegration } from '@/lib/api/contract';
import { qualitySummary } from '@/lib/settings/quality';
import { traktNavSummary } from '@/lib/settings/trakt';
import { cx } from '@/lib/utils/cx';
import { PlaceholderSection } from './PlaceholderSection';
import { PlaybackSection } from './PlaybackSection';
import { QualitySection } from './QualitySection';
import { DEFAULT_SECTION, isSectionId, SECTIONS, type SectionId } from './sections';
import { StatusSection } from './StatusSection';
import { serviceRows, summarize, versionLabel } from './status-model';
import { TraktSection } from './TraktSection';

const GROUPS = ['Integrations', 'Preferences', 'Playback', 'System'] as const;

interface NavData {
  status: SystemStatus | undefined;
  statusFailed: boolean;
  quality: QualityProfile | undefined;
  trakt: TraktIntegration | undefined;
}

function navSummary(id: SectionId, { status, statusFailed, quality, trakt }: NavData): { text: string; attention?: boolean } {
  if (id === 'quality') return { text: quality ? qualitySummary(quality) : '' };
  if (id === 'trakt') return traktNavSummary(trakt);
  if (id === 'status') {
    if (statusFailed) return { text: 'Can’t reach the server', attention: true };
    if (!status) return { text: 'Checking…' };
    const summary = summarize(serviceRows(status, trakt));
    return { text: summary.short, attention: summary.attention };
  }
  if (!status) return { text: '' };
  switch (id) {
    case 'ai':
      return { text: status.ai.configured ? (status.ai.mtd_cost_usd !== null ? `$${status.ai.mtd_cost_usd.toFixed(2)} this month` : 'Key configured') : 'Off' };
    case 'playback':
      return { text: 'WebDAV ready' };
  }
}

/**
 * Settings (screens/settings). Desktop: sidebar + section. Phone: the list
 * on its own, then one section at a time with a way back. `?section=` picks it.
 */
export function SettingsView() {
  const searchParams = useSearchParams();
  const signOut = useSignOut();
  const requested = searchParams.get('section');
  const section = isSectionId(requested) ? requested : null;
  const active = section ?? DEFAULT_SECTION;
  const current = SECTIONS.find((s) => s.id === active)!;
  const { data: status, isError } = useQuery(statusQuery());
  const { data: quality } = useQuery(qualityProfileQuery());
  // The Trakt section polls while a code waits or a sync runs; the nav just reads along
  const { data: trakt } = useQuery({ ...traktIntegrationQuery(), refetchInterval: false });

  return (
    <div className="page py-6 lg:py-8">
      <h1 className={cx('t-title-1 lg:text-[2.5rem]', section && 'hidden lg:block')}>Settings</h1>

      <div className="mt-4 grid gap-8 lg:mt-8 lg:grid-cols-[260px_minmax(0,760px)] lg:gap-12 xl:grid-cols-[280px_minmax(0,820px)] xl:gap-16">
        <nav aria-label="Settings sections" className={cx('flex flex-col gap-5 lg:sticky lg:top-[calc(var(--topbar-h)+24px)] lg:self-start', section && 'hidden lg:flex')}>
          {GROUPS.map((group) => (
            <div key={group} className="flex flex-col gap-1.5">
              <h2 className="t-micro px-3 text-fg-3">{group}</h2>
              <ul className="overflow-hidden rounded-lg border border-line-1 bg-surface-1 max-lg:divide-y max-lg:divide-line-1 lg:border-0 lg:bg-transparent">
                {SECTIONS.filter((s) => s.group === group).map((s) => {
                  const Icon = s.icon;
                  const summary = navSummary(s.id, { status, statusFailed: isError, quality, trakt });
                  const isActive = s.id === active;
                  return (
                    <li key={s.id}>
                      <Link
                        href={`/settings?section=${s.id}`}
                        aria-current={isActive ? 'page' : undefined}
                        className={cx(
                          'flex min-h-[60px] items-center gap-3 px-3 py-2 lg:min-h-[52px] lg:rounded-md',
                          isActive && 'lg:bg-surface-2 lg:shadow-[inset_2px_0_0_var(--accent)]',
                          'hover:bg-surface-2'
                        )}
                      >
                        <span
                          className={cx(
                            'grid size-8 shrink-0 place-items-center rounded-sm bg-surface-2 text-fg-2',
                            isActive && 'lg:bg-accent-soft lg:text-accent-text'
                          )}
                          aria-hidden="true"
                        >
                          <Icon size={17} />
                        </span>
                        <span className="flex min-w-0 grow flex-col">
                          <span className="font-medium text-fg-1">{s.label}</span>
                          {summary.text && <span className="t-small truncate text-fg-3">{summary.text}</span>}
                        </span>
                        {summary.attention && <span className="size-2 shrink-0 rounded-full bg-warning" role="img" aria-label="Needs attention" />}
                        <ChevronRight size={18} className="shrink-0 text-fg-4 lg:hidden" aria-hidden="true" />
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
          <button
            type="button"
            className="flex min-h-[60px] items-center gap-3 rounded-lg border border-line-1 bg-surface-1 px-3 text-left font-medium text-error lg:hidden"
            onClick={() => signOut()}
          >
            <LogOut size={18} aria-hidden="true" /> Log out
          </button>
          <p className="t-small px-3 text-fg-3">
            momoshtrem {status ? `${versionLabel(status.version)} · web ${status.web ? versionLabel(status.web.version) : ''}` : ''}
          </p>
        </nav>

        <section aria-labelledby="section-h" className={cx('set-content flex min-w-0 flex-col gap-5', !section && 'hidden lg:flex')}>
          <Link href="/settings" className="btn btn-ghost btn-sm -ml-3 self-start lg:hidden">
            <ChevronLeft size={16} aria-hidden="true" /> Settings
          </Link>
          <header className="flex flex-col gap-1">
            <h2 id="section-h" className="t-title-1 lg:text-title-2">
              {current.label}
            </h2>
            <p className="text-fg-2">{current.description}</p>
          </header>
          {active === 'trakt' ? (
            <TraktSection />
          ) : active === 'playback' ? (
            <PlaybackSection />
          ) : active === 'status' ? (
            <StatusSection trakt={trakt} />
          ) : active === 'quality' ? (
            <QualitySection />
          ) : (
            <PlaceholderSection id={active} />
          )}
        </section>
      </div>
    </div>
  );
}
